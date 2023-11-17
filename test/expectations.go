package test

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/awslabs/operatorpkg/object"
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

func ExpectToReconcile(ctx context.Context, controller reconcile.Reconciler, object client.Object) reconcile.Result {
	GinkgoHelper()
	result, err := controller.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(object)})
	Expect(err).ToNot(HaveOccurred())
	return result
}

func ExpectToGet[T client.Object](ctx context.Context, c client.Client, obj T) T {
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
				return fmt.Sprintf("expected %s to be deleted, but it still exists", object.GKNN(o))
			})
	}
}

func ExpectApplied(ctx context.Context, c client.Client, objects ...client.Object) {
	GinkgoHelper()
	for _, o := range objects {
		current := o.DeepCopyObject().(client.Object)
		statuscopy := o.DeepCopyObject().(client.Object) // Snapshot the status, since create/update may override
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
		// Update status
		statuscopy.SetResourceVersion(o.GetResourceVersion())
		Expect(c.Status().Update(ctx, statuscopy)).To(Or(Succeed(), MatchError(ContainSubstring("not found")))) // Some objects do not have a status

		// Re-get the object to grab the updated spec and status
		Expect(c.Get(ctx, client.ObjectKeyFromObject(o), o)).To(Succeed())
	}
}
