package object

import (
	"fmt"
	"reflect"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// Scheme for registering object
var Scheme = scheme.Scheme

func ToString(o client.Object) string {
	if o.GetNamespace() == "" {
		return fmt.Sprintf("%s/%s", reflect.TypeOf(o).Elem(), o.GetName())
	}
	return fmt.Sprintf("%s/%s/%s", reflect.TypeOf(o).Elem(), o.GetNamespace(), o.GetName())
}

func GVK(o client.Object) schema.GroupVersionKind {
	return lo.Must(apiutil.GVKForObject(o, Scheme))
}

func OwnedBy(owner client.Object) metav1.ObjectMeta {
	gvk := GVK(owner)
	return metav1.ObjectMeta{
		Name:        owner.GetName(),
		Namespace:   owner.GetNamespace(),
		Annotations: owner.GetAnnotations(),
		Labels:      owner.GetLabels(),
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion:         gvk.GroupVersion().String(),
			Kind:               gvk.Kind,
			Name:               owner.GetName(),
			UID:                owner.GetUID(),
			Controller:         lo.ToPtr(true),
			BlockOwnerDeletion: lo.ToPtr(true),
		}},
	}
}

func Hash(o client.Object) string {
	raw := lo.Must(runtime.DefaultUnstructuredConverter.ToUnstructured(o))
	unstructured.RemoveNestedField(raw, "status")
	unstructured.RemoveNestedField(raw, "metadata")
	unstructured.SetNestedStringMap(raw, o.GetLabels(), "metadata.labels")
	unstructured.SetNestedStringMap(raw, o.GetAnnotations(), "metadata.annotations")
	return fmt.Sprint(lo.Must(hashstructure.Hash(raw, hashstructure.FormatV2, nil)))
}
