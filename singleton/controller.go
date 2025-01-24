package singleton

import (
	"context"
	"math"
	"time"

	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

type ChannalObjectReconciler[T client.Object] interface {
	Reconcile(ctx context.Context, object T) (reconcile.Result, error)
}

func AsChannelObjectReconciler[T client.Object](watchEvents <-chan watch.Event, reconciler ChannalObjectReconciler[T]) reconcile.Reconciler {
	return reconcile.Func(func(ctx context.Context, r reconcile.Request) (reconcile.Result, error) {
		var errs error
		var results []reconcile.Result
		for event := range watchEvents {
			res, err := reconciler.Reconcile(ctx, event.Object.(T))
			errs = multierr.Append(errs, err)
			results = append(results, res)

		}

		var result reconcile.Result
		min := time.Duration(math.MaxInt64)
		for _, r := range results {
			if r.IsZero() {
				continue
			}
			if r.RequeueAfter < min {
				min = r.RequeueAfter
				result.RequeueAfter = min
				result.Requeue = true
			}
		}

		return result, errs
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
