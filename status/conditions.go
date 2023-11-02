package status

import (
	"context"
	"time"

	"github.com/awslabs/operatorpkg/event"
	"github.com/awslabs/operatorpkg/object"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func NewConditions(object Object, conditions []Condition, conditionTypePolarity ConditionTypePolarity) Conditions {
	return Conditions{object: object, conditions: lo.ToPtr(conditions), conditionTypePolarity: conditionTypePolarity}
}

type Conditions struct {
	object                client.Object
	conditions            *[]Condition
	conditionTypePolarity ConditionTypePolarity
}

type ConditionTypePolarity map[ConditionType]ConditionPolarity

func (c Conditions) List() []Condition {
	return *c.conditions
}

func (c Conditions) Get(conditionType ConditionType) *Condition {
	for i := range c.List() {
		if (*c.conditions)[i].Type == conditionType {
			return &(*c.conditions)[i]
		}
	}
	return nil
}

func (c Conditions) Remove(ctx context.Context, conditionType ConditionType) {
	condition := c.Get(conditionType)
	if condition == nil {
		return
	}
	log.FromContext(ctx, "object", object.ToString(c.object), "condition", condition).Info("removed condition")
	*c.conditions = lo.Filter(*c.conditions, func(condition Condition, _ int) bool { return condition.Type == conditionType })
}

func (c Conditions) Set(ctx context.Context, condition Condition) {
	condition.ObservedGeneration = c.object.GetGeneration()

	existing := c.Get(condition.Type)
	if existing == nil {
		condition.LastTransitionTime = metav1.Now()
		*c.conditions = append(*c.conditions, condition)
		return
	}

	isHealthy := bool(
		!c.conditionTypePolarity[condition.Type] && condition.Status == metav1.ConditionTrue ||
			c.conditionTypePolarity[condition.Type] && condition.Status == metav1.ConditionFalse,
	)

	condition.Severity = lo.Ternary(isHealthy, ConditionSeverityInfo, ConditionSeverityError)

	if condition.Status != existing.Status {
		condition.LastTransitionTime = metav1.Now()
		ConditionDuration.
			With(prometheus.Labels{MetricLabelConditionType: string(existing.Type), MetricLabelConditionStatus: string(existing.Status)}).
			Observe(time.Since(existing.LastTransitionTime.Time).Seconds())
		log.FromContext(ctx, "object", object.ToString(c.object), "condition", condition).
			Info("transitioned condition")
		event.FromContext(ctx).Publish(event.Event{
			InvolvedObject: c.object,
			Type:           lo.Ternary(isHealthy, v1.EventTypeNormal, v1.EventTypeWarning),
			Reason:         condition.Reason,
			Message:        condition.Message,
		})
	}
	*existing = condition
}

func (c Conditions) SetTrue(ctx context.Context, conditionType ConditionType) {
	c.Set(ctx, Condition{Type: conditionType, Status: metav1.ConditionTrue, Reason: string(conditionType)})
}

func (c Conditions) SetFalse(ctx context.Context, conditionType ConditionType, reason string, message string) {
	c.Set(ctx, Condition{Type: conditionType, Status: metav1.ConditionFalse, Reason: reason, Message: message})
}
