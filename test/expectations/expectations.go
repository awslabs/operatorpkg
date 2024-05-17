package test

import (
	"context"
	"fmt"
	"time"

	"github.com/awslabs/operatorpkg/object"
	"github.com/awslabs/operatorpkg/status"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	SlowTimeout = 100 * time.Second
	SlowPolling = 10 * time.Second
	FastTimeout = 1 * time.Second
	FastPolling = 10 * time.Millisecond
)

func ExpectObjectReconciled[T client.Object](ctx context.Context, c client.Client, reconciler reconcile.ObjectReconciler[T], object T) reconcile.Result {
	GinkgoHelper()
	result, err := reconcile.AsReconciler(c, reconciler).Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(object)})
	Expect(err).ToNot(HaveOccurred())
	return result
}

func ExpectObjectReconcileFailed[T client.Object](ctx context.Context, c client.Client, reconciler reconcile.ObjectReconciler[T], object T) error {
	GinkgoHelper()
	_, err := reconcile.AsReconciler(c, reconciler).Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(object)})
	Expect(err).To(HaveOccurred())
	return err
}

// Deprecated: Use the more modern ExpectObjectReconciled and reconcile.AsReconciler instead
func ExpectReconciled(ctx context.Context, reconciler reconcile.Reconciler, object client.Object) reconcile.Result {
	GinkgoHelper()
	result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(object)})
	Expect(err).ToNot(HaveOccurred())
	return result
}

func ExpectGet[T client.Object](ctx context.Context, c client.Client, obj T) {
	GinkgoHelper()
	Expect(c.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
}

func ExpectNotFound(ctx context.Context, c client.Client, objects ...client.Object) {
	GinkgoHelper()
	for _, o := range objects {
		Eventually(func() bool { return errors.IsNotFound(c.Get(ctx, client.ObjectKeyFromObject(o), o)) }).
			WithTimeout(FastTimeout).
			WithPolling(FastPolling).
			Should(BeTrue(), func() string {
				return fmt.Sprintf("expected %s to be deleted, but it still exists", object.GVKNN(o))
			})
	}
}

func ExpectApplied(ctx context.Context, c client.Client, objects ...client.Object) {
	GinkgoHelper()
	for _, o := range objects {
		current := o.DeepCopyObject().(client.Object)
		// Create or Update
		if err := c.Get(ctx, client.ObjectKeyFromObject(current), current); err != nil {
			if errors.IsNotFound(err) {
				Expect(c.Create(ctx, o)).To(Succeed())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}
		} else {
			o.SetResourceVersion(current.GetResourceVersion())
			Expect(c.Update(ctx, o)).To(Succeed())
		}

		// Re-get the object to grab the updated spec and status
		ExpectGet(ctx, c, o)
	}
}

func EventuallyExpectStatusConditions(ctx context.Context, c client.Client, obj status.Object, timeout time.Duration, conditions ...status.Condition) {
	Eventually(func(g Gomega) {
		g.Expect(c.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(BeNil())
		objStatus := obj.StatusConditions()
		for _, cond := range conditions {
			objCondition := objStatus.Get(cond.Type)
			g.Expect(objCondition).ToNot(BeNil())
			if cond.Status != "" {
				g.Expect(objCondition.Status).To(Equal(cond.Status))
			}
			if cond.Message != "" {
				g.Expect(objCondition.Message).To(Equal(cond.Message))
			}
			if cond.Reason != "" {
				g.Expect(objCondition.Reason).To(Equal(cond.Reason))
			}
		}
	}).
		WithTimeout(timeout).
		// each polling interval
		WithPolling(timeout / 20).
		Should(Succeed())
}

func ExpectStatusUpdated(ctx context.Context, c client.Client, objects ...client.Object) {
	GinkgoHelper()
	for _, o := range objects {
		// Previous implementations attempted the following:
		// 1. Using merge patch, instead
		// 2. Including this logic in ExpectApplied to simplify test code
		// The former doesn't work, as merge patches cannot reset
		// primitives like strings and integers to "" or 0, and CRDs
		// don't support strategic merge patch. The latter doesn't work
		// since status must be updated in another call, which can cause
		// optimistic locking issues if other threads are updating objects
		// e.g. pod statuses being updated during integration tests.
		Expect(c.Status().Update(ctx, o.DeepCopyObject().(client.Object))).To(Succeed())
		ExpectGet(ctx, c, o)
	}
}

func ExpectDeleted(ctx context.Context, c client.Client, objects ...client.Object) {
	GinkgoHelper()
	for _, o := range objects {
		Expect(c.Delete(ctx, o)).To(Succeed())
		Expect(c.Get(ctx, client.ObjectKeyFromObject(o), o)).To(Or(Succeed(), MatchError(ContainSubstring("not found"))))
	}
}
