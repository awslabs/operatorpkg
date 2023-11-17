package status_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheus "github.com/prometheus/client_model/go"

	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
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
		Expect(fooCondition.Reason).To(Equal(""))
		Expect(fooCondition.Message).To(Equal(""))
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

// GetMetric attempts to find a metric given name and labels
// If no metric is found, the *prometheus.Metric will be nil
func GetMetric(name string, labels ...map[string]string) *prometheus.Metric {
	family, found := lo.Find(lo.Must(metrics.Registry.Gather()), func(family *prometheus.MetricFamily) bool { return family.GetName() == name })
	if !found {
		return nil
	}
	for _, m := range family.Metric {
		temp := lo.Assign(labels...)
		for _, labelPair := range m.Label {
			if v, ok := temp[labelPair.GetName()]; ok && v == labelPair.GetValue() {
				delete(temp, labelPair.GetName())
			}
		}
		if len(temp) == 0 {
			return m
		}
	}
	return nil
}
