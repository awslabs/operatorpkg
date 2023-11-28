package status

import (
	"context"
	"fmt"

	"github.com/awslabs/operatorpkg/object"
	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	MetricLabelGroup           = "group"
	MetricLabelKind            = "kind"
	MetricLabelNamespace       = "namespace"
	MetricLabelName            = "name"
	MetricLabelConditionType   = "type"
	MetricLabelConditionStatus = "status"
)

const (
	MetricNamespace = "operator"
	MetricSubsystem = "status_condition"
)

type Controller struct {
	kubeClient         client.Client
	eventRecorder      record.EventRecorder
	forObject          Object
	observedConditions map[reconcile.Request]ConditionSet
}

func NewController(client client.Client, forObject Object, eventRecorder record.EventRecorder) *Controller {
	return &Controller{
		kubeClient:         client,
		eventRecorder:      eventRecorder,
		forObject:          forObject,
		observedConditions: map[reconcile.Request]ConditionSet{},
	}
}

func (c *Controller) Register(ctx context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		For(c.forObject).
		Named("status").
		Complete(c)
}

func (c *Controller) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	o := c.forObject.DeepCopyObject().(Object)
	gvk := object.GVK(o)
	objectLabels := prometheus.Labels{
		MetricLabelGroup:     gvk.Group,
		MetricLabelKind:      gvk.Kind,
		MetricLabelNamespace: req.Namespace,
		MetricLabelName:      req.Name,
	}

	if err := c.kubeClient.Get(ctx, req.NamespacedName, o); err != nil {
		if errors.IsNotFound(err) {
			ConditionCount.DeletePartialMatch(objectLabels)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("getting object, %w", err)
	}

	currentConditions := o.StatusConditions()
	observedConditions := c.observedConditions[req]
	c.observedConditions[req] = currentConditions

	// Detect and record condition counts
	for _, condition := range o.GetConditions() {
		ConditionCount.MustCurryWith(objectLabels).With(prometheus.Labels{
			MetricLabelConditionType:   string(condition.Type),
			MetricLabelConditionStatus: string(condition.Status),
		}).Set(1)
	}
	for _, observedCondition := range observedConditions.List() {
		if currentCondition := currentConditions.Get(observedCondition.Type); currentCondition == nil || currentCondition.Status != observedCondition.Status {
			ConditionCount.MustCurryWith(objectLabels).Delete(prometheus.Labels{
				MetricLabelConditionType:   string(observedCondition.Type),
				MetricLabelConditionStatus: string(observedCondition.Status),
			})
		}
	}

	// Detect and record status transitions. This approach is best effort,
	// since we may batch multiple writes within a single reconcile loop.
	// It's exceedingly difficult to atomically track all changes to an
	// object, since the Kubernetes is evenutally consistent by design.
	// Despite this, we can catch the majority of transition by remembering
	// what we saw last, and reporting observed changes.
	//
	// We rejected the alternative of tracking these changes within the
	// condition library itself, since you cannot guarantee that a
	// transition made in memory was successfully persisted.
	//
	// Automatic monitoring systems must assume that these observations are
	// lossy, specifically for when a condition transition rapidly. However,
	// for the common case, we want to alert when a transition took a long
	// time, and our likelyhood of observing this is much higher.
	for _, condition := range currentConditions.List() {
		observedCondition := observedConditions.Get(condition.Type)
		if observedCondition == nil || observedCondition.GetStatus() == condition.GetStatus() {
			continue
		}
		duration := condition.LastTransitionTime.Time.Sub(observedCondition.LastTransitionTime.Time).Seconds()
		ConditionDuration.MustCurryWith(objectLabels).With(prometheus.Labels{
			MetricLabelConditionType:   string(observedCondition.Type),
			MetricLabelConditionStatus: string(observedCondition.Status),
		}).Observe(float64(duration))
		c.eventRecorder.Event(o, v1.EventTypeNormal, string(condition.Type), fmt.Sprintf("Status condition transitioned, Type: %s, Status: %s -> %s, Reason: %s, Message: %s",
			condition.Type,
			observedCondition.Status,
			condition.Status,
			condition.Reason,
			condition.Message,
		))
	}
	return reconcile.Result{}, nil
}

// Cardinality is limited to # objects * # conditions * # objectives
var ConditionDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: MetricNamespace,
		Subsystem: MetricSubsystem,
		Name:      "transition_seconds",
		Help:      "The amount of time a condition was in a given state before transitioning. e.g. Alarm := P99(Updated=False) > 5 minutes",
	},
	[]string{
		MetricLabelGroup,
		MetricLabelKind,
		MetricLabelNamespace,
		MetricLabelName,
		MetricLabelConditionType,
		MetricLabelConditionStatus,
	},
)

// Cardinality is limited to # objects * # conditions
var ConditionCount = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: MetricNamespace,
		Subsystem: MetricSubsystem,
		Name:      "count",
		Help:      "The number of an condition for a given object, type and status. e.g. Alarm := Available=False > 0",
	},
	[]string{
		MetricLabelGroup,
		MetricLabelKind,
		MetricLabelNamespace,
		MetricLabelName,
		MetricLabelConditionType,
		MetricLabelConditionStatus,
	},
)

func init() {
	metrics.Registry.MustRegister(
		ConditionCount,
		ConditionDuration,
	)
}
