// Inspired by https://github.com/knative/pkg/tree/97c7258e3a98b81459936bc7a29dc6a9540fa357/apis,
// but we chose to diverge due to the unacceptably large dependency closure of knative/pkg.
package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Object interface {
	client.Object
	GetConditions() []Condition
	SetConditions([]Condition)
	StatusConditions() ConditionSet
}

// ConditionType is a upper-camel-cased condition type.
type ConditionType string

const (
	// ConditionReady specifies that the resource is ready.
	// For long-running resources.
	ConditionReady ConditionType = "Ready"
	// ConditionSucceeded specifies that the resource has finished.
	// For resource which run to completion.
	ConditionSucceeded ConditionType = "Succeeded"
)

// ConditionSeverity expresses the severity of a Condition Type failing.
type ConditionSeverity string

const (
	// ConditionSeverityError specifies that a failure of a condition type
	// should be viewed as an error.  As "Error" is the default for conditions
	// we use the empty string (coupled with omitempty) to avoid confusion in
	// the case where the condition is in state "True" (aka nothing is wrong).
	ConditionSeverityError ConditionSeverity = ""
	// ConditionSeverityInfo specifies that a failure of a condition type
	// should be viewed as purely informational, and that things could still work.
	ConditionSeverityInfo ConditionSeverity = "Info"
)

// Condition defines a readiness condition for a resource.
// See: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
// +k8s:deepcopy-gen=true
type Condition struct {
	// Type of condition.
	// +required
	Type ConditionType `json:"type" description:"type of status condition"`

	// Status of the condition, one of True, False, Unknown.
	// +required
	Status metav1.ConditionStatus `json:"status" description:"status of the condition, one of True, False, Unknown"`

	// Severity with which to treat failures of this type of condition.
	// When this is not specified, it defaults to Error.
	// +optional
	Severity ConditionSeverity `json:"severity,omitempty" description:"how to interpret failures of this condition, one of Error, Warning, Info"`

	// LastTransitionTime is the last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" description:"last time the condition transit from one status to another"`

	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty" description:"one-word CamelCase reason for the condition's last transition"`

	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty" description:"human-readable message indicating details about last transition"`
}

// IsTrue is true if the condition is True
func (c *Condition) IsTrue() bool {
	if c == nil {
		return false
	}
	return c.Status == metav1.ConditionTrue
}

// IsFalse is true if the condition is False
func (c *Condition) IsFalse() bool {
	if c == nil {
		return false
	}
	return c.Status == metav1.ConditionFalse
}

// IsUnknown is true if the condition is Unknown
func (c *Condition) IsUnknown() bool {
	if c == nil {
		return true
	}
	return c.Status == metav1.ConditionUnknown
}

func (c *Condition) GetSeverity() ConditionSeverity {
	if c == nil {
		return ConditionSeverityError
	}
	return c.Severity
}

func (c *Condition) GetStatus() metav1.ConditionStatus {
	if c == nil {
		return metav1.ConditionUnknown
	}
	return c.Status
}

// GetReason returns a nil save string of Reason
func (c *Condition) GetReason() string {
	if c == nil {
		return ""
	}
	return c.Reason
}

// GetMessage returns a nil save string of Message
func (c *Condition) GetMessage() string {
	if c == nil {
		return ""
	}
	return c.Message
}
