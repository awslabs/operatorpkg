package status

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	metricLabelName            = "name"
	metricLabelNamespace       = "namespace"
	metricLabelKind            = "kind"
	metricLabelConditionType   = "type"
	metricLabelConditionStatus = "status"
	metricNamespace            = "operator"
	metricSubsystem            = "status_condition"
)

type ConditionedObject interface {
	v1.Object
	runtime.Object
	GetConditions() Conditions
}

type Conditions []v1.Condition

func (c *Conditions) Set(conditionType string, conditionStatus v1.ConditionStatus, conditionMessage string) {
	condition := c.Get(conditionType)

	if condition == nil {
		*c = append(*c, v1.Condition{
			Type:               conditionType,
			Status:             conditionStatus,
			Message:            conditionMessage,
			Reason:             conditionType,
			LastTransitionTime: v1.Now(),
		})
		return
	}

	if condition.Status != conditionStatus {
		ConditionDuration.
			With(prometheus.Labels{metricLabelConditionType: condition.Type, metricLabelConditionStatus: string(condition.Status)}).
			Observe(time.Since(condition.LastTransitionTime.Time).Seconds())
		condition.LastTransitionTime = v1.Now()
	}

	condition.ObservedGeneration = 0 // TODO, fix this to map to the object
	condition.Message = conditionMessage
	condition.Reason = conditionType // mirror type into reason
	condition.Status = conditionStatus
}

func (c *Conditions) Get(conditionType string) *v1.Condition {
	for i := range *c {
		condition := &[]v1.Condition(*c)[i]
		if condition.Type == conditionType {
			return condition
		}
	}
	return nil
}
