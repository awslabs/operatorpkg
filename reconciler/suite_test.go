package reconciler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/awslabs/operatorpkg/reconciler"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Reconciler")
}

// MockRateLimiter is a mock implementation of workqueue.TypedRateLimiter for testing
type MockRateLimiter[K comparable] struct {
	whenFunc        func(K) time.Duration
	numRequeues     int
	backoffDuration time.Duration
}

func (m *MockRateLimiter[K]) When(key K) time.Duration {
	if m.whenFunc != nil {
		return m.whenFunc(key)
	}
	m.numRequeues++
	return m.backoffDuration
}

func (m *MockRateLimiter[K]) NumRequeues(key K) int {
	return m.numRequeues
}

func (m *MockRateLimiter[K]) Forget(key K) {
	m.numRequeues = 0
}

// MockReconciler is a mock implementation of Reconciler for testing
type MockReconciler struct {
	reconcileFunc func(context.Context) (reconciler.Result, error)
	result        reconciler.Result
	err           error
}

func (m *MockReconciler) Reconcile(ctx context.Context) (reconciler.Result, error) {
	if m.reconcileFunc != nil {
		return m.reconcileFunc(ctx)
	}
	return m.result, m.err
}

var _ = Describe("Reconciler", func() {
	It("should return the original result without backoff", func() {
		backoff := 5 * time.Second
		// Create a mock reconciler
		mockReconciler := &MockReconciler{
			result: reconciler.Result{
				Result: reconcile.Result{
					RequeueAfter: backoff,
				},
				RequeueWithBackoff: false,
			},
		}

		// Create the reconciler adapter
		adapter := reconciler.AsReconciler(mockReconciler)

		// Call the adapter
		ctx := context.Background()
		req := reconcile.Request{}
		result, err := adapter.Reconcile(ctx, req)

		// Verify the result
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(backoff))
	})

	It("should return the original result without backoff when RequeueWithBackoff is not set", func() {
		mockReconciler := &MockReconciler{
			result: reconciler.Result{},
		}

		// Create the reconciler adapter
		adapter := reconciler.AsReconciler(mockReconciler)

		// Call the adapter
		ctx := context.Background()
		req := reconcile.Request{}
		result, err := adapter.Reconcile(ctx, req)

		// Verify the result
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeZero())
	})

	It("should return a result with RequeueAfter set", func() {
		// Create a mock reconciler that returns RequeueWithBackoff = true
		mockReconciler := &MockReconciler{
			result: reconciler.Result{
				Result:             reconcile.Result{},
				RequeueWithBackoff: true,
			},
		}

		// Create the reconciler adapter
		adapter := reconciler.AsReconciler(mockReconciler)

		// Call the adapter
		ctx := context.Background()
		req := reconcile.Request{}
		result, err := adapter.Reconcile(ctx, req)

		// Verify the result - should have some backoff duration
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
	})
	It("should return the error without processing backoff", func() {
		// Create a mock reconciler that returns an error
		expectedErr := errors.New("test error")
		mockReconciler := &MockReconciler{
			result: reconciler.Result{RequeueWithBackoff: true},
			err:    expectedErr,
		}

		// Create the reconciler adapter
		adapter := reconciler.AsReconciler(mockReconciler)

		// Call the adapter
		ctx := context.Background()
		req := reconcile.Request{}
		result, err := adapter.Reconcile(ctx, req)

		// Verify that the error is propagated
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(expectedErr))
		Expect(result.RequeueAfter).To(BeZero())
	})

	It("should use custom rate limiter for backoff", func() {
		backoffDuration := 10 * time.Second
		mockRateLimiter := &MockRateLimiter[reconcile.Request]{
			backoffDuration: backoffDuration,
		}

		// Create a mock reconciler that returns RequeueWithBackoff = true
		mockReconciler := &MockReconciler{
			result: reconciler.Result{
				Result:             reconcile.Result{},
				RequeueWithBackoff: true,
			},
		}

		// Create the reconciler adapter with custom rate limiter
		adapter := reconciler.AsReconcilerWithRateLimiter(mockReconciler, mockRateLimiter)

		// Call the adapter
		ctx := context.Background()
		req := reconcile.Request{}
		result, err := adapter.Reconcile(ctx, req)

		// Verify the result
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(backoffDuration))
		Expect(mockRateLimiter.NumRequeues(req)).To(Equal(1))
	})
})
