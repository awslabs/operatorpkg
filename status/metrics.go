package status

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
)

func RegisterMetrics(registry *prometheus.Registry) {
	registry.MustRegister(
		ConditionCount,
		ConditionDuration,
	)
}

var ConditionDuration = prometheus.NewSummaryVec(
	prometheus.SummaryOpts{
		Namespace:  metricNamespace,
		Subsystem:  metricSubsystem,
		Name:       "transition_seconds",
		Help:       "The amount of time a condition was in a given state before transitioning. e.g. Alarm := P99(Updated=False) > 5 minutes",
		Objectives: lo.SliceToMap([]float64{0.0, 0.5, 0.9, 0.99, 1.0}, func(key float64) (float64, float64) { return key, 0.01 }),
	},
	[]string{
		metricLabelConditionType,
		metricLabelConditionStatus,
	},
)

// Cardinality is limited to # objects * # conditions
var ConditionCount = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Subsystem: metricSubsystem,
		Name:      "count",
		Help:      "The number of an condition for a given object, type and status. e.g. Alarm := Available=False > 0",
	},
	[]string{
		metricLabelKind,
		metricLabelNamespace,
		metricLabelName,
		metricLabelConditionType,
		metricLabelConditionStatus,
	},
)
