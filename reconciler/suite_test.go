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
				Requeue: false,
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
	It("should return the original result without backoff when Requeue is not set", func() {
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
	It("should return a result with Requeue set", func() {
		// Create a mock reconciler that returns Requeue = true
		mockReconciler := &MockReconciler{
			result: reconciler.Result{
				Result:  reconcile.Result{},
				Requeue: true,
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
		Expect(result.RequeueAfter).To(Equal(5 * time.Millisecond))
	})
	It("should return a result with RequeueAfter when both RequeueAfter and Requeue are set", func() {
		// Create a mock reconciler that returns Requeue = true
		mockReconciler := &MockReconciler{
			result: reconciler.Result{
				Result: reconcile.Result{
					RequeueAfter: 10 * time.Second,
				},
				Requeue: true,
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
		Expect(result.RequeueAfter).To(Equal(10 * time.Second))
	})
	It("should return the error without processing backoff", func() {
		// Create a mock reconciler that returns an error
		expectedErr := errors.New("test error")
		mockReconciler := &MockReconciler{
			result: reconciler.Result{Requeue: true},
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
		mockRateLimiter := &MockRateLimiter[reconcile.Request]{
			backoffDuration: 10 * time.Second,
		}

		// Create a mock reconciler that returns Requeue = true
		mockReconciler := &MockReconciler{
			result: reconciler.Result{
				Result:  reconcile.Result{},
				Requeue: true,
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
		Expect(result.RequeueAfter).To(Equal(10 * time.Second))
		Expect(mockRateLimiter.NumRequeues(req)).To(Equal(1))
	})
	It("should implement exponential backoff on repeated calls", func() {
		mockReconciler := &MockReconciler{
			result: reconciler.Result{
				Result:  reconcile.Result{},
				Requeue: true,
			},
		}
		// Multiple calls to the same controller should show increasing delays
		delays := make([]time.Duration, 5)
		reconciler := reconciler.AsReconciler(mockReconciler)

		for i := range 5 {
			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{})
			Expect(err).NotTo(HaveOccurred())
			delays[i] = result.RequeueAfter
		}

		// Verify generally increasing pattern
		initialDelay := 5 * time.Millisecond
		Expect(delays[0]).To(BeNumerically("==", initialDelay))
		for i := 1; i < len(delays); i++ {
			initialDelay *= 2
			Expect(delays[i]).To(BeNumerically("==", initialDelay))
			Expect(delays[i]).To(BeNumerically(">", delays[i-1]),
				"Delay at index %d (%v) should be >= delay at index %d (%v)",
				i, delays[i], i-1, delays[i-1])
		}
	})
	It("should forget an item when reconcile succeeds", func() {
		mockReconciler := &MockReconciler{
			result: reconciler.Result{
				Result:  reconcile.Result{},
				Requeue: false,
			},
		}
		// Multiple calls to the same controller should show zero requeue
		reconciler := reconciler.AsReconciler(mockReconciler)

		for i := 0; i < 5; i++ {
			result, err := reconciler.Reconcile(context.Background(), reconcile.Request{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		}
	})
})
