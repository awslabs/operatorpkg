package status

import (
	"context"
	"fmt"

	"github.com/awslabs/operatorpkg/object"
	"github.com/prometheus/client_golang/prometheus"
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
