package singleton

import (
	"context"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
		GenericFunc: func(_ context.Context, _ event.GenericEvent, queue workqueue.RateLimitingInterface) {
			queue.Add(reconcile.Request{})
		},
	})
}
