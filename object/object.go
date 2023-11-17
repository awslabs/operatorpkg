package object

import (
	"fmt"
	"reflect"

	"dario.cat/mergo"
	"github.com/mitchellh/hashstructure/v2"
	"github.com/samber/lo"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func GKNN(o client.Object) string {
	gvk := GVK(o)
	gknn := fmt.Sprintf("%s.%s", gvk.Kind, gvk.Group)
	if o.GetNamespace() != "" {
		gknn += "/" + o.GetNamespace()
	}
	gknn += "/" + o.GetName()
	return gknn
}

func GVK(o client.Object) schema.GroupVersionKind {
	return lo.Must(apiutil.GVKForObject(o, scheme.Scheme))
}

func WithOwner[T client.Object](owner client.Object, object T) T {
	owned := reflect.New(reflect.TypeOf(object).Elem()).Interface().(T)
	gvk := GVK(owner)
	owned.SetName(owner.GetName())
	owned.SetNamespace(owner.GetNamespace())
	owned.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		Controller:         lo.ToPtr(true),
		BlockOwnerDeletion: lo.ToPtr(true),
	}})
	lo.Must0(mergo.Merge(owned, object, mergo.WithOverride, mergo.WithAppendSlice))
	return owned
}

func Hash(o client.Object) string {
	raw := lo.Must(runtime.DefaultUnstructuredConverter.ToUnstructured(o))
	unstructured.RemoveNestedField(raw, "status")
	unstructured.RemoveNestedField(raw, "metadata")
	unstructured.SetNestedStringMap(raw, o.GetLabels(), "metadata.labels")
	unstructured.SetNestedStringMap(raw, o.GetAnnotations(), "metadata.annotations")
	return fmt.Sprint(lo.Must(hashstructure.Hash(raw, hashstructure.FormatV2, nil)))
}

func Unmarshal[T client.Object](raw []byte) *T {
	t := *new(T)
	lo.Must0(yaml.Unmarshal(raw, &t))
	return &t
}
