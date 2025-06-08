package singleton

import (
	"context"
	"time"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// RequeueImmediately is a constant that allows for immediate RequeueAfter when you want to run your
	// singleton controller as hot as possible in a fast requeuing loop
	RequeueImmediately = 1 * time.Nanosecond
)

type Reconciler interface {
	Reconcile(ctx context.Context) (reconcile.Result, error)
}

func AsReconciler(reconciler Reconciler) reconcile.Reconciler {
	return reconcile.Func(func(ctx context.Context, r reconcile.Request) (reconcile.Result, error) {
		return reconciler.Reconcile(ctx)
	})
}

func Source() source.Source {
	eventSource := make(chan event.GenericEvent, 1)
	eventSource <- event.GenericEvent{}
	return source.Channel(eventSource, handler.Funcs{
		GenericFunc: func(_ context.Context, _ event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			queue.Add(reconcile.Request{})
		},
	})
}

// In response to Requeue: True being deprecated via: https://github.com/kubernetes-sigs/controller-runtime/pull/3107/files
// This uses a bucket and per item delay but the item will be the same because the key is the controller name..
// This implements the same behavior as Requeue: True.
type SingletonRateLimiter struct {
	rateLimiter workqueue.TypedRateLimiter[string]
	key         string
}

func NewSingletonRateLimiter(controllerName string) *SingletonRateLimiter {
	return &SingletonRateLimiter{
		rateLimiter: workqueue.DefaultTypedControllerRateLimiter[string](),
		key:         controllerName,
	}
}

// Delay requeues the item according to the rate limiter.
// Used like "RequeueAfter: controller.RateLimiter.Delay()"
func (s *SingletonRateLimiter) Delay() time.Duration {
	return s.rateLimiter.When(s.key)
}
