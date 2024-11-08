package events

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
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/record"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Controller[T client.Object] struct {
	kind           string
	kubeClient     client.Client
	eventRecorder  record.EventRecorder
	observedEvents sync.Map // map[reconcile.Request]ConditionSet
}

func NewController[T client.Object](client client.Client, eventRecorder record.EventRecorder) *Controller[T] {
	return &Controller[T]{
		kind:          reflect.TypeOf(object.New[T]()).Elem().Name(),
		kubeClient:    client,
		eventRecorder: eventRecorder,
	}
}

func (c *Controller[T]) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		For(object.New[T]()).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Named(fmt.Sprintf("operatorpkg.%s.events", strings.ToLower(c.kind))).
		Complete(c)
}

func (c *Controller[T]) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	o := object.New[T]()
	gvk := object.GVK(o)

	if err := c.kubeClient.Get(ctx, req.NamespacedName, o); err != nil {
		if errors.IsNotFound(err) {
			EventCount.DeletePartialMatch(prometheus.Labels{
				MetricLabelGroup:     gvk.Group,
				MetricLabelKind:      gvk.Kind,
				MetricLabelNamespace: req.Namespace,
				MetricLabelName:      req.Name,
			})
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("getting object, %w", err)
	}

	currentEvents := &v1.EventList{}
	if err := c.kubeClient.List(ctx, currentEvents, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"involvedObject.kind": c.kind,
		}),
	}); err != nil {
		return reconcile.Result{}, err
	}
	observedEvents := []v1.Event{}
	if v, ok := c.observedEvents.Load(req); ok {
		observedEvents = v.([]v1.Event)
	}
	c.observedEvents.Store(req, currentEvents.Items)

	// Detect and record event counts
	for _, event := range currentEvents.Items {
		if event.InvolvedObject.Name == req.Name &&
			event.InvolvedObject.Namespace == req.Namespace {
			EventCount.With(prometheus.Labels{
				MetricLabelGroup:        gvk.Group,
				MetricLabelKind:         gvk.Kind,
				MetricLabelNamespace:    req.Namespace,
				MetricLabelName:         req.Name,
				MetricLabelEventType:    event.Type,
				MetricLabelEventMessage: event.Message,
				MetricLabelEventReason:  event.Reason,
			}).Set(float64(event.Count))
		}
	}

	for _, observedEvent := range observedEvents {
		if observedEvent.InvolvedObject.Name == req.Name &&
			observedEvent.InvolvedObject.Namespace == req.Namespace {
			_, found := lo.Find(currentEvents.Items, func(e v1.Event) bool {
				return e.Type == observedEvent.Type && e.Reason == observedEvent.Reason && e.Message == observedEvent.Message
			})
			if !found {
				EventCount.Delete(prometheus.Labels{
					MetricLabelGroup:        gvk.Group,
					MetricLabelKind:         gvk.Kind,
					MetricLabelNamespace:    req.Namespace,
					MetricLabelName:         req.Name,
					MetricLabelEventType:    observedEvent.Type,
					MetricLabelEventMessage: observedEvent.Message,
					MetricLabelEventReason:  observedEvent.Reason,
				})
			}
		}
	}

	return reconcile.Result{RequeueAfter: time.Second * 10}, nil
}
