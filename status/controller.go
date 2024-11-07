package status

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/awslabs/operatorpkg/object"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
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
	MetricLabelConditionReason = "reason"
)

const (
	MetricNamespace      = "operator"
	MetricSubsystem      = "status_condition"
	TerminationSubsystem = "termination"
)

type Controller[T Object] struct {
	kubeClient         client.Client
	eventRecorder      record.EventRecorder
	observedConditions sync.Map // map[reconcile.Request]ConditionSet
	terminatingObjects sync.Map // map[reconcile.Request]DeletionTimestamp
}

func NewController[T Object](client client.Client, eventRecorder record.EventRecorder) *Controller[T] {
	return &Controller[T]{
		kubeClient:    client,
		eventRecorder: eventRecorder,
	}
}

func (c *Controller[T]) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		For(object.New[T]()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Named(fmt.Sprintf("operatorpkg.%s.status", strings.ToLower(reflect.TypeOf(object.New[T]()).Elem().Name()))).
		Complete(c)
}

func (c *Controller[T]) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	return c.reconcile(ctx, req, object.New[T]())
}

func (c *Controller[T]) reconcile(ctx context.Context, req reconcile.Request, o Object) (reconcile.Result, error) {
	gvk := object.GVK(o)
	if err := c.kubeClient.Get(ctx, req.NamespacedName, o); err != nil {
		if errors.IsNotFound(err) {
			ConditionCount.DeletePartialMatch(prometheus.Labels{
				MetricLabelGroup:     gvk.Group,
				MetricLabelKind:      gvk.Kind,
				MetricLabelNamespace: req.Namespace,
				MetricLabelName:      req.Name,
			})
			ConditionCurrentStatusSeconds.DeletePartialMatch(prometheus.Labels{
				MetricLabelGroup:     gvk.Group,
				MetricLabelKind:      gvk.Kind,
				MetricLabelNamespace: req.Namespace,
				MetricLabelName:      req.Name,
			})
			TerminationCurrentTimeSeconds.DeletePartialMatch(prometheus.Labels{
				MetricLabelNamespace: req.Namespace,
				MetricLabelName:      req.Name,
				MetricLabelGroup:     gvk.Group,
				MetricLabelKind:      gvk.Kind,
			})
			if deletionTS, ok := c.terminatingObjects.Load(req); ok {
				TerminationDuration.With(prometheus.Labels{
					MetricLabelGroup:     gvk.Group,
					MetricLabelKind:      gvk.Kind,
					MetricLabelNamespace: req.Namespace,
					MetricLabelName:      req.Name,
				}).Observe(time.Since(deletionTS.(*metav1.Time).Time).Seconds())
			}
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("getting object, %w", err)
	}

	currentConditions := o.StatusConditions()
	observedConditions := ConditionSet{}
	if v, ok := c.observedConditions.Load(req); ok {
		observedConditions = v.(ConditionSet)
	}
	c.observedConditions.Store(req, currentConditions)

	// Detect and record condition counts
	for _, condition := range o.GetConditions() {
		ConditionCount.With(prometheus.Labels{
			MetricLabelGroup:           gvk.Group,
			MetricLabelKind:            gvk.Kind,
			MetricLabelNamespace:       req.Namespace,
			MetricLabelName:            req.Name,
			MetricLabelConditionType:   condition.Type,
			MetricLabelConditionStatus: string(condition.Status),
			MetricLabelConditionReason: condition.Reason,
		}).Set(1)
		ConditionCurrentStatusSeconds.With(prometheus.Labels{
			MetricLabelGroup:           gvk.Group,
			MetricLabelKind:            gvk.Kind,
			MetricLabelNamespace:       req.Namespace,
			MetricLabelName:            req.Name,
			MetricLabelConditionType:   condition.Type,
			MetricLabelConditionStatus: string(condition.Status),
			MetricLabelConditionReason: condition.Reason,
		}).Set(time.Since(condition.LastTransitionTime.Time).Seconds())
	}
	if o.GetDeletionTimestamp() != nil {
		TerminationCurrentTimeSeconds.With(prometheus.Labels{
			MetricLabelNamespace: req.Namespace,
			MetricLabelName:      req.Name,
			MetricLabelGroup:     gvk.Group,
			MetricLabelKind:      gvk.Kind,
		}).Set(time.Since(o.GetDeletionTimestamp().Time).Seconds())
		c.terminatingObjects.Store(req, o.GetDeletionTimestamp())
	}
	for _, observedCondition := range observedConditions.List() {
		if currentCondition := currentConditions.Get(observedCondition.Type); currentCondition == nil || currentCondition.Status != observedCondition.Status {
			ConditionCount.Delete(prometheus.Labels{
				MetricLabelGroup:           gvk.Group,
				MetricLabelKind:            gvk.Kind,
				MetricLabelNamespace:       req.Namespace,
				MetricLabelName:            req.Name,
				MetricLabelConditionType:   observedCondition.Type,
				MetricLabelConditionStatus: string(observedCondition.Status),
				MetricLabelConditionReason: observedCondition.Reason,
			})
			ConditionCurrentStatusSeconds.Delete(prometheus.Labels{
				MetricLabelGroup:           gvk.Group,
				MetricLabelKind:            gvk.Kind,
				MetricLabelNamespace:       req.Namespace,
				MetricLabelName:            req.Name,
				MetricLabelConditionType:   observedCondition.Type,
				MetricLabelConditionStatus: string(observedCondition.Status),
				MetricLabelConditionReason: observedCondition.Reason,
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
		if observedCondition.GetStatus() == condition.GetStatus() {
			continue
		}
		// A condition transitions if it either didn't exist before or it has changed
		ConditionTransitionsTotal.With(prometheus.Labels{
			MetricLabelGroup:           gvk.Group,
			MetricLabelKind:            gvk.Kind,
			MetricLabelConditionType:   condition.Type,
			MetricLabelConditionStatus: string(condition.Status),
			MetricLabelConditionReason: condition.Reason,
		}).Inc()
		if observedCondition == nil {
			continue
		}
		duration := condition.LastTransitionTime.Time.Sub(observedCondition.LastTransitionTime.Time).Seconds()
		ConditionDuration.With(prometheus.Labels{
			MetricLabelGroup:           gvk.Group,
			MetricLabelKind:            gvk.Kind,
			MetricLabelConditionType:   observedCondition.Type,
			MetricLabelConditionStatus: string(observedCondition.Status),
		}).Observe(duration)
		c.eventRecorder.Event(o, v1.EventTypeNormal, condition.Type, fmt.Sprintf("Status condition transitioned, Type: %s, Status: %s -> %s, Reason: %s%s",
			condition.Type,
			observedCondition.Status,
			condition.Status,
			condition.Reason,
			lo.Ternary(condition.Message != "", fmt.Sprintf(", Message: %s", condition.Message), ""),
		))
	}
	return reconcile.Result{RequeueAfter: time.Second * 10}, nil
}

type GenericObjectController[T client.Object] struct {
	*Controller[UnstructuredAdapter]
}

func NewGenericObjectController[T client.Object](client client.Client, eventRecorder record.EventRecorder) *GenericObjectController[T] {
	return &GenericObjectController[T]{
		Controller: NewController[UnstructuredAdapter](client, eventRecorder),
	}
}

func (c *GenericObjectController[T]) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	return c.reconcile(ctx, req, lo.Must(NewUnstructuredAdapter(object.New[T]())))
}

func (c *GenericObjectController[T]) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		For(object.New[T]()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Named(fmt.Sprintf("operatorpkg.%s.status", strings.ToLower(reflect.TypeOf(object.New[T]()).Elem().Name()))).
		Complete(c)
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
// This metric is useful for aggreations. If you need a high accuracy metric, use operator_status_condition_last_transition_time_seconds
var ConditionCurrentStatusSeconds = prometheus.NewGaugeVec(
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
var ConditionTransitionsTotal = prometheus.NewCounterVec(
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

var TerminationCurrentTimeSeconds = prometheus.NewGaugeVec(
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

var TerminationDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: MetricNamespace,
		Subsystem: TerminationSubsystem,
		Name:      "duration_seconds",
		Help:      "The amount of time taken by an object to terminate completely.",
	},
	[]string{
		MetricLabelGroup,
		MetricLabelKind,
		MetricLabelNamespace,
		MetricLabelName,
	},
)

func init() {
	metrics.Registry.MustRegister(
		ConditionCount,
		ConditionDuration,
		ConditionTransitionsTotal,
		ConditionCurrentStatusSeconds,
		TerminationCurrentTimeSeconds,
		TerminationDuration,
	)
}
