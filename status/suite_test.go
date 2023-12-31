package status_test

import (
	"testing"

	"github.com/awslabs/operatorpkg/status"
	"github.com/awslabs/operatorpkg/test"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
)

func Test(t *testing.T) {
	lo.Must0(SchemeBuilder.AddToScheme(scheme.Scheme))
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Status")
}

var (
	SchemeBuilder = runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(schema.GroupVersion{Group: test.APIGroup, Version: "v1alpha1"}, &TestObject{})
		return nil
	})
)

// +k8s:deepcopy-gen=true
// +kubebuilder:object:root=true
type TestObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            TestStatus `json:"status"`
}

// +k8s:deepcopy-gen=true
type TestStatus struct {
	Conditions []status.Condition `json:"conditions,omitempty"`
}

const (
	// Normal Conditions
	ConditionTypeFoo = "Foo"
	ConditionTypeBar = "Bar"
	// Abnormal Conditions
	ConditionTypeBaz = "Baz"
)

func (t *TestObject) StatusConditions() status.ConditionSet {
	return status.NewReadyConditions(ConditionTypeFoo, ConditionTypeBar).For(t)
}

func (t *TestObject) GetConditions() []status.Condition {
	return t.Status.Conditions
}

func (t *TestObject) SetConditions(conditions []status.Condition) {
	t.Status.Conditions = conditions
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestObject) DeepCopyInto(out *TestObject) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestObject.
func (in *TestObject) DeepCopy() *TestObject {
	if in == nil {
		return nil
	}
	out := new(TestObject)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TestObject) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TestStatus) DeepCopyInto(out *TestStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]status.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TestStatus.
func (in *TestStatus) DeepCopy() *TestStatus {
	if in == nil {
		return nil
	}
	out := new(TestStatus)
	in.DeepCopyInto(out)
	return out
}
