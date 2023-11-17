package test

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/Pallinder/go-randomdata"
	"github.com/imdario/mergo"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	APIGroup             = "operators.k8s.aws"
	DiscoveryLabel       = APIGroup + "/test-id"
	sequentialNumber     = 0
	sequentialNumberLock = new(sync.Mutex)
)

var Namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}

func Object[T client.Object](base T, overrides ...T) T {
	dest := reflect.New(reflect.TypeOf(base).Elem()).Interface().(T)
	dest.SetName(RandomName())
	dest.SetNamespace(Namespace.Name)
	dest.SetLabels(lo.Assign(dest.GetLabels(), map[string]string{DiscoveryLabel: dest.GetName()}))
	for _, src := range append([]T{base}, overrides...) {
		lo.Must0(mergo.Merge(dest, src, mergo.WithOverride))
	}
	return dest
}

func RandomName() string {
	sequentialNumberLock.Lock()
	defer sequentialNumberLock.Unlock()
	sequentialNumber++
	return strings.ToLower(fmt.Sprintf("%s-%d-%s", randomdata.SillyName(), sequentialNumber, randomdata.Alphanumeric(10)))
}
