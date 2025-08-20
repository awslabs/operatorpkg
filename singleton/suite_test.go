package singleton_test

import (
	"context"
	"testing"

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

var mockReconciler *MockReconciler

// MockReconciler for testing
type MockReconciler struct {
	result reconciler.Result
	err    error
}

func (m *MockReconciler) Reconcile(ctx context.Context) (reconciler.Result, error) {
	return m.result, m.err
}

var _ = Describe("Singleton Controller", func() {
	Context("AsReconciler", func() {
		BeforeEach(func() {
			mockReconciler = &MockReconciler{}
		})
		It("should return a result with RequeueAfter that is scoped to a controller", func() {
			// Test with different controllers to ensure they're handled independently
			controller1 := &MockReconciler{
				result: reconciler.Result{Requeue: true},
			}
			controller2 := &MockReconciler{
				result: reconciler.Result{Requeue: true},
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
			Expect(result1.RequeueAfter).To(Equal(result2.RequeueAfter))
		})
	})
})
