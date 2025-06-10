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

// KeyExtractor extracts a rate limiter key from a context and reconcile.Request.
type KeyExtractor[K any] interface {
	Extract(ctx context.Context, req reconcile.Request) K
}

// RequestKeyExtractor extracts a reconcile.Request key for rate limiting.
type RequestKeyExtractor struct{}

// Extract returns the reconcile.Request as the key.
func (e RequestKeyExtractor) Extract(ctx context.Context, req reconcile.Request) reconcile.Request {
	return req
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
	return AsGenericReconciler(
		func(ctx context.Context, req reconcile.Request) (Result, error) {
			return reconciler.Reconcile(ctx)
		},
		RequestKeyExtractor{},
	)
}

// AsGenericReconciler creates a reconciler with a specific key extractor
func AsGenericReconciler[K comparable](
	reconcileFunc func(ctx context.Context, req reconcile.Request) (Result, error),
	keyExtractor KeyExtractor[K],
) reconcile.Reconciler {
	return AsGenericReconcilerWithRateLimiter(
		reconcileFunc,
		keyExtractor,
		workqueue.DefaultTypedControllerRateLimiter[K](),
	)
}

// AsGenericReconcilerWithRateLimiter creates a reconciler with a custom rate limiter
func AsGenericReconcilerWithRateLimiter[K comparable](
	reconcileFunc func(ctx context.Context, req reconcile.Request) (Result, error),
	keyExtractor KeyExtractor[K],
	rateLimiter workqueue.TypedRateLimiter[K],
) reconcile.Reconciler {
	return reconcile.Func(func(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
		result, err := reconcileFunc(ctx, req)
		if err != nil {
			return reconcile.Result{}, err
		}
		if result.RequeueWithBackoff {
			return reconcile.Result{RequeueAfter: rateLimiter.When(keyExtractor.Extract(ctx, req))}, nil
		}
		return result.Result, nil
	})
}
