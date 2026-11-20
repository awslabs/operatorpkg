package status_test

import (
	"testing"

	"github.com/awslabs/operatorpkg/test"
)

// readyObject returns an object whose root and dependent conditions are all set, as most objects are once reconciled.
func readyObject() *test.CustomObject {
	obj := test.Object(&test.CustomObject{})
	obj.StatusConditions().SetTrue(test.ConditionTypeFoo)
	obj.StatusConditions().SetTrue(test.ConditionTypeBar)
	return obj
}

// b.Loop stops the compiler from inlining calls in the loop body, so each benchmark calls a helper. Callers in
// controllers are ordinary functions like these, where the ConditionSet methods do get inlined.

func getIsTrue(obj *test.CustomObject) bool {
	return obj.StatusConditions().Get(test.ConditionTypeFoo).IsTrue()
}

func isTrue(obj *test.CustomObject) bool {
	return obj.StatusConditions().IsTrue(test.ConditionTypeFoo, test.ConditionTypeBar)
}

func rootIsTrue(obj *test.CustomObject) bool {
	return obj.StatusConditions().Root().IsTrue()
}

func BenchmarkStatusConditionsGet(b *testing.B) {
	obj := readyObject()
	b.ReportAllocs()
	for b.Loop() {
		_ = getIsTrue(obj)
	}
}

func BenchmarkStatusConditionsIsTrue(b *testing.B) {
	obj := readyObject()
	b.ReportAllocs()
	for b.Loop() {
		_ = isTrue(obj)
	}
}

func BenchmarkStatusConditionsRoot(b *testing.B) {
	obj := readyObject()
	b.ReportAllocs()
	for b.Loop() {
		_ = rootIsTrue(obj)
	}
}
