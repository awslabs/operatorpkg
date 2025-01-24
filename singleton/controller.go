package singleton

import (
	"context"
	"time"

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

func Source() source.Source {
	eventSource := make(chan event.GenericEvent, 1)
	eventSource <- event.GenericEvent{}
	return source.Channel(eventSource, handler.Funcs{
		GenericFunc: func(_ context.Context, _ event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			queue.Add(reconcile.Request{})
		},
	})
}

func ChannelSource[T client.Object](ctx context.Context, objectChan <-chan watch.Event) source.Source {
	eventSource := make(chan event.GenericEvent, 1000)

	go func(ctx context.Context, objectChan <-chan watch.Event, eventSource chan event.GenericEvent) {
		for {
			select {
			case delta, ok := <-objectChan:
				if !ok {
					return
				}
				// Convert the delta to GenericEvent
				eventSource <- event.GenericEvent{
					Object: delta.Object.(T),
				}
			case <-ctx.Done():
				return
			}
		}
	}(ctx, objectChan, eventSource)

	return source.Channel(eventSource, handler.Funcs{
		GenericFunc: func(_ context.Context, event event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			queue.Add(reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(event.Object),
			})
		},
	})
}
