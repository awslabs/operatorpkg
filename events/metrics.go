package events

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	MetricLabelGroup        = "group"
	MetricLabelKind         = "kind"
	MetricLabelNamespace    = "namespace"
	MetricLabelName         = "name"
	MetricLabelEventType    = "type"
	MetricLabelEventMessage = "message"
	MetricLabelEventReason  = "reason"
)

const (
	MetricNamespace = "operator"
	MetricSubsystem = "event"
)

// Cardinality is limited to # objects * # conditions
var EventCount = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: MetricNamespace,
		Subsystem: MetricSubsystem,
		Name:      "count",
		Help:      "The number of an event for a given object, reason and message. e.g. Alarm := Available=False > 0",
	},
	[]string{
		MetricLabelNamespace,
		MetricLabelName,
		MetricLabelGroup,
		MetricLabelKind,
		MetricLabelEventType,
		MetricLabelEventReason,
		MetricLabelEventMessage,
	},
)

func init() {
	metrics.Registry.MustRegister(
		EventCount,
	)
}
