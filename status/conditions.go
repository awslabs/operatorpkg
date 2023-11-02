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

func NewConditions(object Object, conditions []Condition) Conditions {
	return Conditions{object: object, conditions: lo.ToPtr(conditions)}
}

type Conditions struct {
	object     client.Object
	conditions *[]Condition
}

func (c Conditions) Set(ctx context.Context, condition Condition) {
	condition.ObservedGeneration = c.object.GetGeneration()

	existing := c.Get(condition.Type)
	if existing == nil {
		condition.LastTransitionTime = metav1.Now()
		*c.conditions = append(*c.conditions, condition)
		return
	}
	if condition.Status != existing.Status {
		condition.LastTransitionTime = metav1.Now()
		// Record a metric
		ConditionDuration.
			With(prometheus.Labels{MetricLabelConditionType: existing.Type, MetricLabelConditionStatus: string(existing.Status)}).
			Observe(time.Since(existing.LastTransitionTime.Time).Seconds())
		// Write a log
		log.FromContext(ctx,
			"object", object.ToString(c.object),
			"condition", condition,
		).Info("status transitioned")
		// Emit an event
		event.FromContext(ctx).Publish(event.Event{
			InvolvedObject: c.object,
			Type:           lo.Ternary(condition.Severity == ConditionSeverityInfo, v1.EventTypeNormal, v1.EventTypeWarning),
			Reason:         condition.Reason,
			Message:        condition.Message,
		})
	}
	*existing = condition
}

func (c Conditions) Get(conditionType string) *Condition {
	for i := range c.List() {
		if (*c.conditions)[i].Type == conditionType {
			return &(*c.conditions)[i]
		}
	}
	return nil
}

func (c Conditions) List() []Condition {
	return *c.conditions
}
