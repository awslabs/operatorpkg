package reconciler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/awslabs/operatorpkg/reconciler"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MockRateLimiter is a mock implementation of workqueue.TypedRateLimiter for testing
type MockRateLimiter[K comparable] struct {
	whenFunc       func(K) time.Duration
	numRequeues    int
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

// MockKeyExtractor is a mock implementation of KeyExtractor for testing
type MockKeyExtractor[K any] struct {
	extractFunc func(context.Context, reconcile.Request) K
	key         K
}

func (m *MockKeyExtractor[K]) Extract(ctx context.Context, req reconcile.Request) K {
	if m.extractFunc != nil {
		return m.extractFunc(ctx, req)
	}
	return m.key
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

func TestResult(t *testing.T) {
	t.Run("Basic functionality", func(t *testing.T) {
		t.Run("Without backoff", func(t *testing.T) {
			// Create a Result with RequeueWithBackoff = false and a specific RequeueAfter duration
			duration := 5 * time.Second
			result := reconciler.Result{
				Result: reconcile.Result{
					RequeueAfter: duration,
				},
				RequeueWithBackoff: false,
			}

			// Verify that the wrapped Result has the same RequeueAfter value
			assert.Equal(t, duration, result.RequeueAfter)
			assert.False(t, result.RequeueWithBackoff)
		})

		t.Run("With requeue", func(t *testing.T) {
			// Create a Result with Requeue = true and RequeueWithBackoff = false
			result := reconciler.Result{
				Result: reconcile.Result{
					Requeue: true,
				},
				RequeueWithBackoff: false,
			}

			// Verify that the wrapped Result has Requeue = true
			assert.True(t, result.Requeue)
			assert.False(t, result.RequeueWithBackoff)
		})
	})
}

func TestAsGenericReconciler(t *testing.T) {
	t.Run("With string key", func(t *testing.T) {
		t.Run("Result with backoff", func(t *testing.T) {
			// Create a mock rate limiter
			backoffDuration := 10 * time.Second
			mockRateLimiter := &MockRateLimiter[string]{
				backoffDuration: backoffDuration,
			}

			// Create a mock reconcile function that returns a Result with RequeueWithBackoff = true
			reconcileFunc := func(ctx context.Context, req reconcile.Request) (reconciler.Result, error) {
				return reconciler.Result{
					Result:            reconcile.Result{},
					RequeueWithBackoff: true,
				}, nil
			}

			// Create a mock key extractor
			testKey := "test-controller"
			mockKeyExtractor := &MockKeyExtractor[string]{
				key: testKey,
			}

			// Create the reconciler adapter
			adapter := reconciler.AsGenericReconcilerWithRateLimiter(
				reconcileFunc,
				mockKeyExtractor,
				mockRateLimiter,
			)

			// Call the adapter
			ctx := context.Background()
			req := reconcile.Request{}
			result, err := adapter.Reconcile(ctx, req)

			// Verify the result
			assert.NoError(t, err)
			assert.Equal(t, backoffDuration, result.RequeueAfter)
			assert.False(t, result.Requeue)
			assert.Equal(t, 1, mockRateLimiter.NumRequeues(testKey))
		})

		t.Run("Multiple backoffs", func(t *testing.T) {
			// Create a mock rate limiter that increases backoff duration
			initialBackoff := 1 * time.Second
			mockLimiter := &MockRateLimiter[string]{}
			mockLimiter.whenFunc = func(key string) time.Duration {
				mockLimiter.numRequeues++
				return time.Duration(mockLimiter.numRequeues) * initialBackoff
			}

			// Create a mock reconcile function that returns a Result with RequeueWithBackoff = true
			reconcileFunc := func(ctx context.Context, req reconcile.Request) (reconciler.Result, error) {
				return reconciler.Result{
					Result:            reconcile.Result{},
					RequeueWithBackoff: true,
				}, nil
			}

			// Create a mock key extractor
			testKey := "test-controller"
			mockKeyExtractor := &MockKeyExtractor[string]{
				key: testKey,
			}

			// Create the reconciler adapter
			adapter := reconciler.AsGenericReconcilerWithRateLimiter(
				reconcileFunc,
				mockKeyExtractor,
				mockLimiter,
			)

			// Call the adapter multiple times
			ctx := context.Background()
			req := reconcile.Request{}
			
			// First call
			result1, err := adapter.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, 1*initialBackoff, result1.RequeueAfter)
			
			// Second call
			result2, err := adapter.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, 2*initialBackoff, result2.RequeueAfter)
			
			// Third call
			result3, err := adapter.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, 3*initialBackoff, result3.RequeueAfter)
		})
	})

	t.Run("With request key", func(t *testing.T) {
		t.Run("Result with backoff", func(t *testing.T) {
			// Create a mock rate limiter
			backoffDuration := 10 * time.Second
			mockRateLimiter := &MockRateLimiter[reconcile.Request]{
				backoffDuration: backoffDuration,
			}

			// Create a mock reconcile function that returns a Result with RequeueWithBackoff = true
			reconcileFunc := func(ctx context.Context, req reconcile.Request) (reconciler.Result, error) {
				return reconciler.Result{
					Result:            reconcile.Result{},
					RequeueWithBackoff: true,
				}, nil
			}

			// Create a mock key extractor
			testReq := reconcile.Request{}
			mockKeyExtractor := &MockKeyExtractor[reconcile.Request]{
				key: testReq,
			}

			// Create the reconciler adapter
			adapter := reconciler.AsGenericReconcilerWithRateLimiter(
				reconcileFunc,
				mockKeyExtractor,
				mockRateLimiter,
			)

			// Call the adapter
			ctx := context.Background()
			result, err := adapter.Reconcile(ctx, testReq)

			// Verify the result
			assert.NoError(t, err)
			assert.Equal(t, backoffDuration, result.RequeueAfter)
			assert.False(t, result.Requeue)
			assert.Equal(t, 1, mockRateLimiter.NumRequeues(testReq))
		})
	})

	t.Run("Error handling", func(t *testing.T) {
		t.Run("Error propagation", func(t *testing.T) {
			// Create a mock reconcile function that returns an error
			expectedErr := errors.New("test error")
			reconcileFunc := func(ctx context.Context, req reconcile.Request) (reconciler.Result, error) {
				return reconciler.Result{}, expectedErr
			}

			// Create a mock key extractor
			mockKeyExtractor := &MockKeyExtractor[string]{
				key: "test-controller",
			}

			// Create the reconciler adapter
			adapter := reconciler.AsGenericReconciler(
				reconcileFunc,
				mockKeyExtractor,
			)

			// Call the adapter
			ctx := context.Background()
			req := reconcile.Request{}
			_, err := adapter.Reconcile(ctx, req)

			// Verify that the error is propagated
			assert.Error(t, err)
			assert.Equal(t, expectedErr, err)
		})
	})

	t.Run("No backoff", func(t *testing.T) {
		t.Run("Result without backoff", func(t *testing.T) {
			// Create a mock reconcile function that returns a Result with RequeueWithBackoff = false
			expectedRequeueAfter := 5 * time.Second
			reconcileFunc := func(ctx context.Context, req reconcile.Request) (reconciler.Result, error) {
				return reconciler.Result{
					Result: reconcile.Result{
						RequeueAfter: expectedRequeueAfter,
					},
					RequeueWithBackoff: false,
				}, nil
			}

			// Create a mock key extractor
			mockKeyExtractor := &MockKeyExtractor[string]{
				key: "test-controller",
			}

			// Create the reconciler adapter
			adapter := reconciler.AsGenericReconciler(
				reconcileFunc,
				mockKeyExtractor,
			)

			// Call the adapter
			ctx := context.Background()
			req := reconcile.Request{}
			result, err := adapter.Reconcile(ctx, req)

			// Verify the result
			assert.NoError(t, err)
			assert.Equal(t, expectedRequeueAfter, result.RequeueAfter)
			assert.False(t, result.Requeue)
		})
	})
}

func TestKeyExtractors(t *testing.T) {
	t.Run("RequestKeyExtractor", func(t *testing.T) {
		t.Run("Extract request key", func(t *testing.T) {
			// Create a request key extractor
			extractor := reconciler.RequestKeyExtractor{}

			// Extract the key
			ctx := context.Background()
			req := reconcile.Request{}
			key := extractor.Extract(ctx, req)

			// Verify the key
			assert.Equal(t, req, key)
		})
	})
}

func TestAsReconciler(t *testing.T) {
	t.Run("Standard reconciler adapter", func(t *testing.T) {
		// Create a mock reconciler
		mockReconciler := &MockReconciler{
			result: reconciler.Result{
				Result: reconcile.Result{
					RequeueAfter: 5 * time.Second,
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
		assert.NoError(t, err)
		assert.Equal(t, 5*time.Second, result.RequeueAfter)
		assert.False(t, result.Requeue)
	})
}

func TestReconcilerFunc(t *testing.T) {
	t.Run("Implementation", func(t *testing.T) {
		// Create a ReconcilerFunc
		expectedResult := reconciler.Result{
			Result: reconcile.Result{
				RequeueAfter: 5 * time.Second,
			},
			RequeueWithBackoff: false,
		}
		expectedErr := errors.New("test error")
		
		reconcileFunc := reconciler.ReconcilerFunc(func(ctx context.Context) (reconciler.Result, error) {
			return expectedResult, expectedErr
		})

		// Call the function
		ctx := context.Background()
		result, err := reconcileFunc.Reconcile(ctx)

		// Verify the result
		assert.Equal(t, expectedResult, result)
		assert.Equal(t, expectedErr, err)
	})
}
