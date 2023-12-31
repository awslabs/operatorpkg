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
		// Conditions should be initialized
		conditions := testObject.StatusConditions()
		Expect(conditions.Get(ConditionTypeFoo).GetStatus()).To(Equal(metav1.ConditionUnknown))
		Expect(conditions.Get(ConditionTypeBar).GetStatus()).To(Equal(metav1.ConditionUnknown))
		Expect(conditions.Root().GetStatus()).To(Equal(metav1.ConditionUnknown))
		// Update the condition
		conditions.SetTrue(ConditionTypeFoo)
		fooCondition := conditions.Get(ConditionTypeFoo)
		Expect(fooCondition.Type).To(Equal(ConditionTypeFoo))
		Expect(fooCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(fooCondition.Reason).To(Equal(ConditionTypeFoo))  // default to type
		Expect(fooCondition.Message).To(Equal(ConditionTypeFoo)) // default to type
		Expect(fooCondition.LastTransitionTime.UnixNano()).To(BeNumerically(">", 0))
		Expect(conditions.Root().GetStatus()).To(Equal(metav1.ConditionUnknown)) // Root condition is still unknown
		time.Sleep(1 * time.Nanosecond)
		// Update the other condition
		conditions.SetTrueWithReason(ConditionTypeBar, "reason", "message")
		fooCondition2 := conditions.Get(ConditionTypeBar)
		Expect(fooCondition2.Type).To(Equal(ConditionTypeBar))
		Expect(fooCondition2.Status).To(Equal(metav1.ConditionTrue))
		Expect(fooCondition2.Reason).To(Equal("reason"))
		Expect(fooCondition2.Message).To(Equal("message"))
		Expect(fooCondition.LastTransitionTime.UnixNano()).To(BeNumerically(">", 0))
		Expect(conditions.Root().GetStatus()).To(Equal(metav1.ConditionTrue)) // Root condition automatically set to true
		time.Sleep(1 * time.Nanosecond)
		// transition the condition
		conditions.SetFalse(ConditionTypeFoo, "reason", "message")
		updatedFooCondition := conditions.Get(ConditionTypeFoo)
		Expect(updatedFooCondition.Type).To(Equal(ConditionTypeFoo))
		Expect(updatedFooCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(updatedFooCondition.Reason).To(Equal("reason"))
		Expect(updatedFooCondition.Message).To(Equal("message"))
		Expect(updatedFooCondition.LastTransitionTime.UnixNano()).To(BeNumerically(">", fooCondition.LastTransitionTime.UnixNano()))
		Expect(conditions.Root().GetStatus()).To(Equal(metav1.ConditionFalse)) // Root condition automatically set to false
		time.Sleep(1 * time.Nanosecond)
		// Transition if the status is the same, but the Reason is different
		conditions.SetFalse(ConditionTypeBar, "another-reason", "another-message")
		updatedBarCondition := conditions.Get(ConditionTypeBar)
		Expect(updatedBarCondition.Type).To(Equal(ConditionTypeBar))
		Expect(updatedBarCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(updatedBarCondition.Reason).To(Equal("another-reason"))
		Expect(updatedBarCondition.LastTransitionTime.UnixNano()).ToNot(BeNumerically("==", fooCondition2.LastTransitionTime.UnixNano()))
	})
})
