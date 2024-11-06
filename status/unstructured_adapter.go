package status

import (
	"encoding/json"

	"github.com/awslabs/operatorpkg/object"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// unstructuredAdapter is an adapter for the status.Object interface. unstructuredAdapter
// makes the assumption that status conditions are found on status.conditions path.
type unstructuredAdapter struct {
	unstructured.Unstructured
}

func NewUnstructuredAdapter(obj client.Object) *unstructuredAdapter {
	u := unstructured.Unstructured{Object: lo.Must(runtime.DefaultUnstructuredConverter.ToUnstructured(obj))}
	u.SetGroupVersionKind(object.GVK(obj))
	return &unstructuredAdapter{Unstructured: u}
}

func (u *unstructuredAdapter) GetConditions() []Condition {
	conditions, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	return lo.Map(conditions, func(condition interface{}, _ int) Condition {
		var newCondition Condition
		cond := condition.(map[string]interface{})
		jsonStr, _ := json.Marshal(cond)
		json.Unmarshal(jsonStr, &newCondition)
		return newCondition
	})
}
func (u *unstructuredAdapter) SetConditions(conditions []Condition) {
	unstructured.SetNestedSlice(u.Object, lo.Map(conditions, func(condition Condition, _ int) interface{} {
		var b map[string]interface{}
		j, _ := json.Marshal(&condition)
		json.Unmarshal(j, &b)
		return b
	}), "status", "conditions")
}

func (u *unstructuredAdapter) StatusConditions() ConditionSet {
	conditionTypes := lo.Map(u.GetConditions(), func(condition Condition, _ int) string {
		return condition.Type
	})
	return NewReadyConditions(conditionTypes...).For(u)
}
