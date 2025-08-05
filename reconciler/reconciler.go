package reconciler

import (
	"context"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Result is a wrapper around reconcile.Result that adds RequeueWithBackoff functionality.
type Result struct {
	reconcile.Result
	RequeueWithBackoff bool
}

// Reconciler defines the interface for standard reconcilers
type Reconciler interface {
	Reconcile(ctx context.Context) (Result, error)
}

// ReconcilerFunc is a function type that implements the Reconciler interface.
type ReconcilerFunc func(ctx context.Context) (Result, error)

// Reconcile implements the Reconciler interface.
func (f ReconcilerFunc) Reconcile(ctx context.Context) (Result, error) {
	return f(ctx)
}

// AsReconciler creates a reconciler from a standard reconciler
func AsReconciler(reconciler Reconciler) reconcile.Reconciler {
	return AsReconcilerWithRateLimiter(
		reconciler,
		workqueue.DefaultTypedControllerRateLimiter[reconcile.Request](),
	)
}

// AsReconcilerWithRateLimiter creates a reconciler with a specific key extractor
func AsReconcilerWithRateLimiter(
	reconciler Reconciler,
	rateLimiter workqueue.TypedRateLimiter[reconcile.Request],
) reconcile.Reconciler {
	return reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
		result, err := reconciler.Reconcile(ctx)
		if err != nil {
			return reconcile.Result{}, err
		}
		if result.RequeueWithBackoff {
			return reconcile.Result{RequeueAfter: rateLimiter.When(req)}, nil
		}
		if result.RequeueAfter > 0 {
			return reconcile.Result{RequeueAfter: result.RequeueAfter}, nil
		}
		return result.Result, nil
	})
}
