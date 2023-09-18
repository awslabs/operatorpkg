package status

import (
	"context"
	"fmt"

	"github.com/awslabs/operatorpkg/object"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Controller struct {
	kubeClient client.Client
	forObject  ConditionedObject
}

func NewController(client client.Client, forObject ConditionedObject) *Controller {
	return &Controller{
		kubeClient: client,
		forObject:  forObject,
	}
}

func (c *Controller) Register(ctx context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		For(c.forObject).
		Named(fmt.Sprintf("metrics.%s", object.GVK(c.forObject).Kind)).
		Complete(c)
}

func (c *Controller) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	o := c.forObject.DeepCopyObject().(ConditionedObject)
	labels := prometheus.Labels{
		metricLabelKind:      object.GVK(o).Kind,
		metricLabelNamespace: req.Namespace,
		metricLabelName:      req.Name,
	}
	ConditionCount.DeletePartialMatch(labels)

	if err := c.kubeClient.Get(ctx, req.NamespacedName, o); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	for _, condition := range o.GetConditions() {
		ConditionCount.MustCurryWith(labels).With(prometheus.Labels{
			metricLabelConditionType:   condition.Type,
			metricLabelConditionStatus: string(condition.Status),
		}).Set(1)
	}
	return reconcile.Result{}, nil
}

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
