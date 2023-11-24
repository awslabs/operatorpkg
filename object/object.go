package object

import (
	"fmt"

	"github.com/samber/lo"
	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// GroupVersionKindNamespacedName uniquely identifies an object
type GroupVersionKindNamespacedName struct {
	schema.GroupVersionKind
	types.NamespacedName
}

// GVKNN returns a GroupVersionKindNamespacedName that uniquely identifies the object
func GVKNN(o client.Object) GroupVersionKindNamespacedName {
	return GroupVersionKindNamespacedName{
		GroupVersionKind: GVK(o),
		NamespacedName:   client.ObjectKeyFromObject(o),
	}
}

func (gvknn GroupVersionKindNamespacedName) String() string {
	str := fmt.Sprintf("%s/%s", gvknn.Group, gvknn.Kind)
	if gvknn.Namespace != "" {
		str += "/" + gvknn.Namespace
	}
	str += "/" + gvknn.Name
	return str
}

func GVK(o client.Object) schema.GroupVersionKind {
	return lo.Must(apiutil.GVKForObject(o, scheme.Scheme))
}

func Unmarshal[T client.Object](raw []byte) *T {
	t := *new(T)
	lo.Must0(yaml.Unmarshal(raw, &t))
	return &t
}
