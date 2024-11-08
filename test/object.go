package test

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/Pallinder/go-randomdata"
	"github.com/awslabs/operatorpkg/status"
	"github.com/imdario/mergo"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

// +k8s:deepcopy-gen=true
// +kubebuilder:object:root=true
type CustomObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            CustomStatus `json:"status"`
}

// +k8s:deepcopy-gen=true
type CustomStatus struct {
	Conditions []status.Condition `json:"conditions,omitempty"`
}

const (
	// Normal Conditions
	ConditionTypeFoo = "Foo"
	ConditionTypeBar = "Bar"
	// Abnormal Conditions
	ConditionTypeBaz = "Baz"
)

func (t *CustomObject) StatusConditions() status.ConditionSet {
	return status.NewReadyConditions(ConditionTypeFoo, ConditionTypeBar).For(t)
}

func (t *CustomObject) GetConditions() []status.Condition {
	return t.Status.Conditions
}

func (t *CustomObject) SetConditions(conditions []status.Condition) {
	t.Status.Conditions = conditions
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CustomObject) DeepCopyInto(out *CustomObject) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CustomObject.
func (in *CustomObject) DeepCopy() *CustomObject {
	if in == nil {
		return nil
	}
	out := new(CustomObject)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CustomObject) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CustomStatus) DeepCopyInto(out *CustomStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]status.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CustomStatus.
func (in *CustomStatus) DeepCopy() *CustomStatus {
	if in == nil {
		return nil
	}
	out := new(CustomStatus)
	in.DeepCopyInto(out)
	return out
}
