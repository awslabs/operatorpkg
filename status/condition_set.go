// Inspired by https://github.com/knative/pkg/tree/97c7258e3a98b81459936bc7a29dc6a9540fa357/apis,
// but we chose to diverge due to the unacceptably large dependency closure of knative/pkg.
package status

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionTypes is an abstract collection of the possible ConditionType values
// that a particular resource might expose.  It also holds the "root condition"
// for that resource, which we define to be one of Ready or Succeeded depending
// on whether it is a Living or Batch process respectively.
type ConditionTypes struct {
	root       ConditionType
	dependents []ConditionType
}

// NewLivingConditions returns a ConditionTypes to hold the conditions for the
// living resource. ConditionReady is used as the root condition.
// The set of condition types provided are those of the terminal subconditions.
func NewLivingConditions(d ...ConditionType) ConditionTypes {
	return newConditionTypes(ConditionReady, d...)
}

// NewBatchConditions returns a ConditionTypes to hold the conditions for the
// batch resource. ConditionSucceeded is used as the happy condition.
// The set of condition types provided are those of the terminal subconditions.
func NewBatchConditions(d ...ConditionType) ConditionTypes {
	return newConditionTypes(ConditionSucceeded, d...)
}

func newConditionTypes(happy ConditionType, dependents ...ConditionType) ConditionTypes {
	return ConditionTypes{
		root:       happy,
		dependents: lo.Reject(lo.Uniq(dependents), func(c ConditionType, _ int) bool { return c == happy }),
	}
}

// ConditionSet provides methods for evaluating Conditions.
// +k8s:deepcopy-gen=false
type ConditionSet struct {
	ConditionTypes
	object Object
}

// For creates a ConditionSet from an object using the original
// ConditionTypes as a reference. Status must be a pointer to a struct.
func (r ConditionTypes) For(object Object) ConditionSet {
	cs := ConditionSet{object: object, ConditionTypes: r}
	// Set known conditions Unknown if not set.
	for _, t := range append(r.dependents, r.root) {
		if cs.Get(t) == nil {
			cs.Set(Condition{
				Type:     t,
				Status:   lo.Ternary(cs.Root().IsTrue(), metav1.ConditionTrue, metav1.ConditionUnknown),
				Severity: cs.severity(t),
			})
		}
	}
	return cs
}

// Root looks returns the root Condition
func (c ConditionSet) Root() *Condition {
	return c.Get(c.root)
}

func (c ConditionSet) List() []Condition {
	return c.object.GetConditions()
}

// GetCondition finds and returns the Condition that matches the ConditionType
// previously set on Conditions.
func (c ConditionSet) Get(t ConditionType) *Condition {
	if c.object == nil {
		return nil
	}
	if condition, found := lo.Find(c.object.GetConditions(), func(c Condition) bool { return c.Type == t }); found {
		return &condition
	}
	return nil
}

// Set sets or updates the Condition on Conditions for Condition.Type.
// If there is an update, Conditions are stored back sorted.
func (c ConditionSet) Set(cond Condition) {
	if c.object == nil {
		return
	}
	t := cond.Type
	var conditions []Condition
	for _, c := range c.object.GetConditions() {
		if c.Type != t {
			conditions = append(conditions, c)
		} else {
			// If we'd only update the LastTransitionTime, then return.
			cond.LastTransitionTime = c.LastTransitionTime
			if reflect.DeepEqual(cond, c) {
				return
			}
		}
	}
	cond.LastTransitionTime = metav1.Now()
	conditions = append(conditions, cond)
	// Sorted for convenience of the consumer, i.e. kubectl.
	sort.Slice(conditions, func(i, j int) bool { return conditions[i].Type < conditions[j].Type })
	c.object.SetConditions(conditions)
}

func (c ConditionSet) isNormal(t ConditionType) bool {
	return t == c.root || lo.Contains(c.dependents, t)
}

func (c ConditionSet) severity(t ConditionType) ConditionSeverity {
	return lo.Ternary(c.isNormal(t), ConditionSeverityError, ConditionSeverityInfo)
}

// RemoveCondition removes the non terminal condition that matches the ConditionType
// Not implemented for terminal conditions
func (c ConditionSet) Clear(t ConditionType) error {
	var conditions []Condition

	if c.object == nil {
		return nil
	}
	// Terminal conditions are not handled as they can't be nil
	if c.isNormal(t) {
		return fmt.Errorf("clearing terminal conditions not implemented")
	}
	cond := c.Get(t)
	if cond == nil {
		return nil
	}
	for _, c := range c.object.GetConditions() {
		if c.Type != t {
			conditions = append(conditions, c)
		}
	}

	// Sorted for convenience of the consumer, i.e. kubectl.
	sort.Slice(conditions, func(i, j int) bool { return conditions[i].Type < conditions[j].Type })
	c.object.SetConditions(conditions)

	return nil
}

// SetTrue sets the status of t to true, and then marks the happy condition to
// true if all other dependents are also true.
func (c ConditionSet) SetTrue(t ConditionType) {
	c.SetTrueWithReason(t, "", "")
	c.recomputeRootCondition(t)
}

// SetTrueWithReason sets the status of t to true with the reason, and then marks the happy condition to
// true if all other dependents are also true.
func (c ConditionSet) SetTrueWithReason(t ConditionType, reason, message string) {
	c.Set(Condition{
		Type:     t,
		Status:   metav1.ConditionTrue,
		Reason:   reason,
		Message:  message,
		Severity: c.severity(t),
	})
	c.recomputeRootCondition(t)
}

// recomputeRootCondition marks the happy condition to true if all other dependents are also true.
func (r ConditionSet) recomputeRootCondition(t ConditionType) {
	if c := r.findUnhappyDependent(); c != nil {
		// Propagate unhappy dependent to happy condition.
		r.Set(Condition{
			Type:     r.root,
			Status:   c.Status,
			Reason:   c.Reason,
			Message:  c.Message,
			Severity: r.severity(r.root),
		})
	} else if t != r.root {
		// Set the happy condition to true.
		r.Set(Condition{
			Type:     r.root,
			Status:   metav1.ConditionTrue,
			Severity: r.severity(r.root),
		})
	}
}

func (c ConditionSet) findUnhappyDependent() *Condition {
	// This only works if there are dependents.
	if len(c.dependents) == 0 {
		return nil
	}

	// Do not modify the objects condition order.
	conditions := append([]Condition{}, c.object.GetConditions()...)

	// Filter based on terminal status.
	n := 0
	for _, condition := range conditions {
		if condition.Severity == ConditionSeverityError && condition.Type != c.root {
			conditions[n] = condition
			n++
		}
	}
	conditions = conditions[:n]

	// Sort set conditions by time.
	sort.Slice(conditions, func(i, j int) bool {
		return conditions[i].LastTransitionTime.After(conditions[j].LastTransitionTime.Time)
	})

	// First check the conditions with Status == False.
	if c, found := lo.Find(conditions, func(c Condition) bool { return c.IsFalse() }); found {
		return &c
	}
	// Second check for conditions with Status == Unknown.
	if c, found := lo.Find(conditions, func(c Condition) bool { return c.IsUnknown() }); found {
		return &c
	}
	// If something was not initialized.
	if len(c.dependents) > len(conditions) {
		return &Condition{
			Status: metav1.ConditionUnknown,
		}
	}
	// All dependents are fine.
	return nil
}

// SetUnknown sets the status of t to Unknown and also sets the happy condition
// to Unknown if no other dependent condition is in an error state.
func (r ConditionSet) SetUnknown(t ConditionType, reason, message string) {
	// set the specified condition
	r.Set(Condition{
		Type:     t,
		Status:   metav1.ConditionUnknown,
		Reason:   reason,
		Message:  message,
		Severity: r.severity(t),
	})

	// check the dependents.
	isDependent := false
	for _, cond := range r.dependents {
		c := r.Get(cond)
		// Failed conditions trump Unknown conditions
		if c.IsFalse() {
			// Double check that the happy condition is also false.
			happy := r.Get(r.root)
			if !happy.IsFalse() {
				r.SetFalse(r.root, reason, message)
			}
			return
		}
		if cond == t {
			isDependent = true
		}
	}

	if isDependent {
		// set the root condition, if it is one of our dependent subconditions.
		r.Set(Condition{
			Type:     r.root,
			Status:   metav1.ConditionUnknown,
			Reason:   reason,
			Message:  message,
			Severity: r.severity(r.root),
		})
	}
}

// SetFalse sets the status of t and the root condition to False.
func (r ConditionSet) SetFalse(t ConditionType, reason, message string) {
	types := []ConditionType{t}
	for _, cond := range r.dependents {
		if cond == t {
			types = append(types, r.root)
		}
	}

	for _, t := range types {
		r.Set(Condition{
			Type:     t,
			Status:   metav1.ConditionFalse,
			Reason:   reason,
			Message:  message,
			Severity: r.severity(t),
		})
	}
}
