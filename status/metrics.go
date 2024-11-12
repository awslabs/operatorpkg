package status

import (
	pmetrics "github.com/awslabs/operatorpkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	MetricLabelGroup           = "group"
	MetricLabelKind            = "kind"
	MetricLabelNamespace       = "namespace"
	MetricLabelName            = "name"
	MetricLabelConditionType   = "type"
	MetricLabelConditionStatus = "status"
	MetricLabelConditionReason = "reason"
)

const (
	MetricNamespace      = "operator"
	MetricSubsystem      = "status_condition"
	TerminationSubsystem = "termination"
)

// Cardinality is limited to # objects * # conditions * # objectives
var ConditionDuration = pmetrics.NewPrometheusHistogram(
	metrics.Registry,
	prometheus.HistogramOpts{
		Namespace: MetricNamespace,
		Subsystem: MetricSubsystem,
		Name:      "transition_seconds",
		Help:      "The amount of time a condition was in a given state before transitioning. e.g. Alarm := P99(Updated=False) > 5 minutes",
	},
	[]string{
		MetricLabelGroup,
		MetricLabelKind,
		MetricLabelConditionType,
		MetricLabelConditionStatus,
	},
)

// Cardinality is limited to # objects * # conditions
var ConditionCount = pmetrics.NewPrometheusGauge(
	metrics.Registry,
	prometheus.GaugeOpts{
		Namespace: MetricNamespace,
		Subsystem: MetricSubsystem,
		Name:      "count",
		Help:      "The number of an condition for a given object, type and status. e.g. Alarm := Available=False > 0",
	},
	[]string{
		MetricLabelNamespace,
		MetricLabelName,
		MetricLabelGroup,
		MetricLabelKind,
		MetricLabelConditionType,
		MetricLabelConditionStatus,
		MetricLabelConditionReason,
	},
)

// Cardinality is limited to # objects * # conditions
// NOTE: This metric is based on a requeue so it won't show the current status seconds with extremely high accuracy.
// This metric is useful for aggregations. If you need a high accuracy metric, use operator_status_condition_last_transition_time_seconds
var ConditionCurrentStatusSeconds = pmetrics.NewPrometheusGauge(
	metrics.Registry,
	prometheus.GaugeOpts{
		Namespace: MetricNamespace,
		Subsystem: MetricSubsystem,
		Name:      "current_status_seconds",
		Help:      "The current amount of time in seconds that a status condition has been in a specific state. Alarm := P99(Updated=Unknown) > 5 minutes",
	},
	[]string{
		MetricLabelNamespace,
		MetricLabelName,
		MetricLabelGroup,
		MetricLabelKind,
		MetricLabelConditionType,
		MetricLabelConditionStatus,
		MetricLabelConditionReason,
	},
)

// Cardinality is limited to # objects * # conditions
var ConditionTransitionsTotal = pmetrics.NewPrometheusCounter(
	metrics.Registry,
	prometheus.CounterOpts{
		Namespace: MetricNamespace,
		Subsystem: MetricSubsystem,
		Name:      "transitions_total",
		Help:      "The count of transitions of a given object, type and status.",
	},
	[]string{
		MetricLabelGroup,
		MetricLabelKind,
		MetricLabelConditionType,
		MetricLabelConditionStatus,
		MetricLabelConditionReason,
	},
)

var TerminationCurrentTimeSeconds = pmetrics.NewPrometheusGauge(
	metrics.Registry,
	prometheus.GaugeOpts{
		Namespace: MetricNamespace,
		Subsystem: TerminationSubsystem,
		Name:      "current_time_seconds",
		Help:      "The current amount of time in seconds that an object has been in terminating state.",
	},
	[]string{
		MetricLabelNamespace,
		MetricLabelName,
		MetricLabelGroup,
		MetricLabelKind,
	},
)

var TerminationDuration = pmetrics.NewPrometheusHistogram(
	metrics.Registry,
	prometheus.HistogramOpts{
		Namespace: MetricNamespace,
		Subsystem: TerminationSubsystem,
		Name:      "duration_seconds",
		Help:      "The amount of time taken by an object to terminate completely.",
	},
	[]string{
		MetricLabelGroup,
		MetricLabelKind,
	},
)
