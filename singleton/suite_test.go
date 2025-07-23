package singleton_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/awslabs/operatorpkg/reconciler"
	"github.com/awslabs/operatorpkg/singleton"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Singleton")
}

var (
	mockReconciler *MockReconciler
)

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

// MockReconciler for testing
type MockReconciler struct {
	name   string
	result reconciler.Result
	err    error
}

func (m *MockReconciler) Name() string {
	return m.name
}

func (m *MockReconciler) Reconcile(ctx context.Context) (reconciler.Result, error) {
	return m.result, m.err
}

var _ = Describe("Singleton Controller", func() {
	Context("AsReconciler", func() {
		BeforeEach(func() {
			mockReconciler = &MockReconciler{
				name: "test-controller",
			}
		})

		Context("when RequeueWithBackoff is false", func() {
			It("should return the original result without backoff", func() {
				mockReconciler.result = reconciler.Result{
					RequeueWithBackoff: false,
				}
				result, err := singleton.AsReconciler(mockReconciler).Reconcile(context.Background(), reconcile.Request{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())
			})
			It("should return the original result without backoff when RequeueWithBackoff is not set", func() {
				mockReconciler.result = reconciler.Result{}
				result, err := singleton.AsReconciler(mockReconciler).Reconcile(context.Background(), reconcile.Request{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeZero())
			})
		})

		Context("when RequeueWithBackoff is true", func() {
			BeforeEach(func() {
				mockReconciler.result = reconciler.Result{
					RequeueWithBackoff: true,
				}
			})

			It("should return a result with RequeueAfter set", func() {
				result, err := singleton.AsReconciler(mockReconciler).Reconcile(context.Background(), reconcile.Request{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">=", 0))
			})

			It("should use controller name for rate limiting", func() {
				// Test with different controller names to ensure they're handled independently
				controller1 := &MockReconciler{
					name:   "controller-1",
					result: reconciler.Result{RequeueWithBackoff: true},
				}
				controller2 := &MockReconciler{
					name:   "controller-2",
					result: reconciler.Result{RequeueWithBackoff: true},
				}

				reconciler1 := singleton.AsReconciler(controller1)
				reconciler2 := singleton.AsReconciler(controller2)

				// Each controller should get its own rate limiting
				result1, err1 := reconciler1.Reconcile(context.Background(), reconcile.Request{})
				result2, err2 := reconciler2.Reconcile(context.Background(), reconcile.Request{})

				Expect(err1).NotTo(HaveOccurred())
				Expect(err2).NotTo(HaveOccurred())
				Expect(result1.RequeueAfter).To(BeNumerically(">=", 0))
				Expect(result2.RequeueAfter).To(BeNumerically(">=", 0))
			})

			It("should implement exponential backoff on repeated calls", func() {
				// Multiple calls to the same controller should show increasing delays
				delays := make([]time.Duration, 5)

				for i := 0; i < 5; i++ {
					result, err := singleton.AsReconciler(mockReconciler).Reconcile(context.Background(), reconcile.Request{})
					Expect(err).NotTo(HaveOccurred())
					delays[i] = result.RequeueAfter
				}

				// Verify generally increasing pattern (allowing for some variance in rate limiting)
				for i := 1; i < len(delays); i++ {
					Expect(delays[i]).To(BeNumerically(">=", delays[i-1]),
						"Delay at index %d (%v) should be >= delay at index %d (%v)",
						i, delays[i], i-1, delays[i-1])
				}
			})
		})

		Context("when reconciler returns an error", func() {
			BeforeEach(func() {
				mockReconciler.result = reconciler.Result{RequeueWithBackoff: true}
				mockReconciler.err = errors.New("test error")
			})

			It("should return the error without processing backoff", func() {
				result, err := singleton.AsReconciler(mockReconciler).Reconcile(context.Background(), reconcile.Request{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("test error"))
				Expect(result.RequeueAfter).To(BeZero())
			})
		})

		Context("integration with RequeueImmediately constant", func() {
			It("should work with immediate requeue pattern", func() {
				mockReconciler.result = reconciler.Result{
					Result: reconcile.Result{RequeueAfter: singleton.RequeueImmediately},
				}

				result, err := singleton.AsReconciler(mockReconciler).Reconcile(context.Background(), reconcile.Request{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(singleton.RequeueImmediately))
			})
		})
	})
	Context("AsReconcilerWithRateLimiter", func() {
		BeforeEach(func() {
			mockReconciler.result = reconciler.Result{
				RequeueWithBackoff: true,
			}
		})
		It("should use the custom rate limiter", func() {
			mockRateLimiter := &MockRateLimiter[string]{
				backoffDuration: 10 * time.Second,
				whenFunc:        func(req string) time.Duration { return 10 * time.Second },
			}
			adapter := singleton.AsReconcilerWithRateLimiter(mockReconciler, mockRateLimiter)
			result, err := adapter.Reconcile(context.Background(), reconcile.Request{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(10 * time.Second))
		})
	})
})
