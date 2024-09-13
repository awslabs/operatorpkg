package status_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Conditions", func() {
	It("should correctly toggle conditions", func() {
		testObject := TestObject{}
		testObject.Generation = 1
		// Conditions should be initialized
		conditions := testObject.StatusConditions()
		Expect(conditions.Get(ConditionTypeFoo).GetStatus()).To(Equal(metav1.ConditionUnknown))
		Expect(conditions.Get(ConditionTypeBar).GetStatus()).To(Equal(metav1.ConditionUnknown))
		Expect(conditions.Root().GetStatus()).To(Equal(metav1.ConditionUnknown))
		// Update the condition to unknown with reason
		Expect(conditions.SetUnknownWithReason(ConditionTypeFoo, "reason", "message")).To(BeTrue())
		fooCondition := conditions.Get(ConditionTypeFoo)
		Expect(fooCondition.Type).To(Equal(ConditionTypeFoo))
		Expect(fooCondition.Status).To(Equal(metav1.ConditionUnknown))
		Expect(fooCondition.Reason).To(Equal("reason"))   // default to type
		Expect(fooCondition.Message).To(Equal("message")) // default to type
		Expect(fooCondition.LastTransitionTime.UnixNano()).To(BeNumerically(">", 0))
		Expect(fooCondition.ObservedGeneration).To(Equal(int64(1)))
		Expect(conditions.Root().GetStatus()).To(Equal(metav1.ConditionUnknown))
		time.Sleep(1 * time.Nanosecond)
		// Update the condition to true
		Expect(conditions.SetTrue(ConditionTypeFoo)).To(BeTrue())
		fooCondition = conditions.Get(ConditionTypeFoo)
		Expect(fooCondition.Type).To(Equal(ConditionTypeFoo))
		Expect(fooCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(fooCondition.Reason).To(Equal(ConditionTypeFoo)) // default to type
		Expect(fooCondition.Message).To(Equal(""))              // default to type
		Expect(fooCondition.LastTransitionTime.UnixNano()).To(BeNumerically(">", 0))
		Expect(fooCondition.ObservedGeneration).To(Equal(int64(1)))
		Expect(conditions.Root().GetStatus()).To(Equal(metav1.ConditionUnknown))
		time.Sleep(1 * time.Nanosecond)
		// Update the other condition to false
		Expect(conditions.SetFalse(ConditionTypeBar, "reason", "message")).To(BeTrue())
		fooCondition2 := conditions.Get(ConditionTypeBar)
		Expect(fooCondition2.Type).To(Equal(ConditionTypeBar))
		Expect(fooCondition2.Status).To(Equal(metav1.ConditionFalse))
		Expect(fooCondition2.Reason).To(Equal("reason"))
		Expect(fooCondition2.Message).To(Equal("message"))
		Expect(fooCondition2.LastTransitionTime.UnixNano()).To(BeNumerically(">", 0))
		Expect(fooCondition2.ObservedGeneration).To(Equal(int64(1)))
		Expect(conditions.Root().GetStatus()).To(Equal(metav1.ConditionFalse))
		time.Sleep(1 * time.Nanosecond)
		// transition the root condition to true
		Expect(conditions.SetTrueWithReason(ConditionTypeBar, "reason", "message")).To(BeTrue())
		updatedFooCondition := conditions.Get(ConditionTypeBar)
		Expect(updatedFooCondition.Type).To(Equal(ConditionTypeBar))
		Expect(updatedFooCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(updatedFooCondition.Reason).To(Equal("reason"))
		Expect(updatedFooCondition.Message).To(Equal("message"))
		Expect(updatedFooCondition.LastTransitionTime.UnixNano()).To(BeNumerically(">", fooCondition.LastTransitionTime.UnixNano()))
		Expect(updatedFooCondition.ObservedGeneration).To(Equal(int64(1)))
		Expect(conditions.Root().GetStatus()).To(Equal(metav1.ConditionTrue))
		time.Sleep(1 * time.Nanosecond)
		// Transition if the status is the same, but the Reason is different
		Expect(conditions.SetFalse(ConditionTypeBar, "another-reason", "another-message")).To(BeTrue())
		updatedBarCondition := conditions.Get(ConditionTypeBar)
		Expect(updatedBarCondition.Type).To(Equal(ConditionTypeBar))
		Expect(updatedBarCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(updatedBarCondition.Reason).To(Equal("another-reason"))
		Expect(updatedBarCondition.LastTransitionTime.UnixNano()).ToNot(BeNumerically("==", fooCondition2.LastTransitionTime.UnixNano()))
		Expect(updatedBarCondition.ObservedGeneration).To(Equal(int64(1)))
		// Dont transition if reason and message are the same
		Expect(conditions.SetTrue(ConditionTypeFoo)).To(BeFalse())
		Expect(conditions.SetFalse(ConditionTypeBar, "another-reason", "another-message")).To(BeFalse())
		testObject.Generation = 2
		Expect(conditions.SetFalse(ConditionTypeBar, "another-reason", "another-message")).To(BeTrue())
		updatedBarCondition2 := conditions.Get(ConditionTypeBar)
		Expect(updatedBarCondition2.LastTransitionTime.UnixNano()).To(BeNumerically(">", updatedBarCondition.LastTransitionTime.UnixNano()))
		Expect(updatedBarCondition2.ObservedGeneration).To(Equal(int64(2)))
		// root should be false when any dependent condition is false
		Expect(conditions.Root().GetStatus()).To(Equal(metav1.ConditionFalse))
		Expect(conditions.Root().Reason).To(Equal("UnhealthyDependents"))
		// root should be unknown when no dependent condition is false and any observedGeneration doesn't match with latest generation
		Expect(conditions.SetTrue(ConditionTypeBar)).To(BeTrue())
		Expect(conditions.Root().GetStatus()).To(Equal(metav1.ConditionUnknown))
		Expect(conditions.Root().Reason).To(Equal("ReconcilingDependents"))
		Expect(conditions.SetTrue(ConditionTypeFoo)).To(BeTrue())
		Expect(conditions.Root().GetStatus()).To(Equal(metav1.ConditionTrue))
	})

	It("all true", func() {
		testObject := TestObject{}
		Expect(testObject.StatusConditions().IsTrue()).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeBar)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeBaz)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBar)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBaz)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBar, ConditionTypeBaz)).To(BeFalse())

		testObject.StatusConditions().SetFalse(ConditionTypeBaz, "reason", "message")
		Expect(testObject.StatusConditions().IsTrue()).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeBar)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeBaz)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBar)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBaz)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBar, ConditionTypeBaz)).To(BeFalse())

		testObject.StatusConditions().SetTrue(ConditionTypeFoo)
		Expect(testObject.StatusConditions().IsTrue()).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo)).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeBar)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeBaz)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBar)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBaz)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBar, ConditionTypeBaz)).To(BeFalse())

		testObject.StatusConditions().SetTrue(ConditionTypeBar)
		Expect(testObject.StatusConditions().IsTrue()).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo)).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeBar)).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeBaz)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBar)).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBaz)).To(BeFalse())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBar, ConditionTypeBaz)).To(BeFalse())

		testObject.StatusConditions().SetTrue(ConditionTypeBaz)
		Expect(testObject.StatusConditions().IsTrue()).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo)).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeBar)).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeBaz)).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBar)).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBaz)).To(BeTrue())
		Expect(testObject.StatusConditions().IsTrue(ConditionTypeFoo, ConditionTypeBar, ConditionTypeBaz)).To(BeTrue())
	})
})
