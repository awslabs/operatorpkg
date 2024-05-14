package test

import (
	"context"
	"fmt"
	"reflect"
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

func ExpectObjectReconciled[T client.Object](ctx context.Context, reconciler reconcile.ObjectReconciler[T], object T) reconcile.Result {
	GinkgoHelper()
	result, err := reconciler.Reconcile(ctx, object)
	Expect(err).ToNot(HaveOccurred())
	return result
}

func ExpectObjectReconcileFailed[T client.Object](ctx context.Context, reconciler reconcile.ObjectReconciler[T], object T) error {
	GinkgoHelper()
	_, err := reconciler.Reconcile(ctx, object)
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

func ExpectGet[T client.Object](ctx context.Context, c client.Client, obj T) T {
	GinkgoHelper()
	resp := reflect.New(reflect.TypeOf(*new(T)).Elem()).Interface().(T)
	Expect(c.Get(ctx, client.ObjectKeyFromObject(obj), resp)).To(Succeed())
	return resp
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
		Expect(c.Get(ctx, client.ObjectKeyFromObject(o), o)).To(Succeed())
	}
}

func ExpectStatusConditions(obj status.Object, conditions ...status.Condition) {
	GinkgoHelper()
	objStatus := obj.StatusConditions()
	for _, cond := range conditions {
		objCondition := objStatus.Get(cond.Type)
		Expect(objCondition).ToNot(BeNil())
		if objCondition.Status != "" {
			Expect(objCondition.Status).To(Equal(cond.Status))
		}
		if objCondition.Message != "" {
			Expect(objCondition.Message).To(Equal(cond.Message))
		}
		if objCondition.Reason != "" {
			Expect(objCondition.Reason).To(Equal(cond.Reason))
		}
	}
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
		Expect(c.Get(ctx, client.ObjectKeyFromObject(o), o)).To(Succeed())
	}
}

func ExpectDeleted(ctx context.Context, c client.Client, objects ...client.Object) {
	GinkgoHelper()
	for _, o := range objects {
		Expect(c.Delete(ctx, o)).To(Succeed())
		Expect(c.Get(ctx, client.ObjectKeyFromObject(o), o)).To(Or(Succeed(), MatchError(ContainSubstring("not found"))))
	}
}
