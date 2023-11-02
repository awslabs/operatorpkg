package status_test

import (
	"time"

	"github.com/awslabs/operatorpkg/event"
	"github.com/awslabs/operatorpkg/status"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheus "github.com/prometheus/client_model/go"

	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

type TestObject struct {
	v1.Pod
	Status TestStatus
}

type TestStatus struct {
	Conditions []status.Condition `json:"conditions,omitempty"`
}

func (t *TestObject) StatusConditions() status.Conditions {
	return status.NewConditions(t, t.Status.Conditions, status.ConditionTypePolarity{
		status.ConditionReady: status.ConditionAbnormalTrue,
	})
}

var _ = Describe("Conditions", func() {

	var mockRecorder *event.MockRecorder
	BeforeEach(func() {
		mockRecorder = event.NewMockRecorder()
		ctx = event.IntoContext(ctx, mockRecorder)
	})

	It("should correctly toggle conditions", func() {
		testObject := TestObject{}
		// Condition is not set
		conditions := testObject.StatusConditions()
		Expect(conditions.Get("foo")).To(BeNil())
		Expect(testObject.Status).To(Equal(TestStatus{}))
		// Update the condition
		conditions.Set(ctx, status.Condition{Type: "foo", Status: metav1.ConditionTrue, Reason: "reason"})
		fooCondition := conditions.Get("foo")
		Expect(fooCondition.Type).To(Equal(status.ConditionType("foo")))
		Expect(fooCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(fooCondition.Reason).To(Equal("reason"))
		Expect(fooCondition.LastTransitionTime.UnixNano()).To(BeNumerically(">", 0))
		time.Sleep(1 * time.Nanosecond)
		// Update another condition
		conditions.Set(ctx, status.Condition{Type: "bar", Status: metav1.ConditionTrue, Reason: "reason"})
		barCondition := conditions.Get("bar")
		Expect(barCondition.Type).To(Equal(status.ConditionType("bar")))
		Expect(barCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(barCondition.Reason).To(Equal("reason"))
		Expect(barCondition.LastTransitionTime.UnixNano()).To(BeNumerically(">", 0))
		time.Sleep(1 * time.Nanosecond)
		// transition the condition
		conditions.Set(ctx, status.Condition{Type: "foo", Status: metav1.ConditionFalse, Reason: "reason"})
		updatedFooCondition := conditions.Get("foo")
		Expect(updatedFooCondition.Type).To(Equal(status.ConditionType("foo")))
		Expect(updatedFooCondition.Status).To(Equal(metav1.ConditionFalse))
		Expect(updatedFooCondition.Reason).To(Equal("reason"))
		Expect(updatedFooCondition.LastTransitionTime.UnixNano()).To(BeNumerically(">", fooCondition.LastTransitionTime.UnixNano()))
		time.Sleep(1 * time.Nanosecond)
		// Don't transition if the status is the same, but the Reason is different
		conditions.Set(ctx, status.Condition{Type: "bar", Status: metav1.ConditionTrue, Reason: "another-reason"})
		updatedBarCondition := conditions.Get("bar")
		Expect(updatedBarCondition.Type).To(Equal(status.ConditionType("bar")))
		Expect(updatedBarCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(updatedBarCondition.Reason).To(Equal("another-reason"))
		Expect(updatedBarCondition.LastTransitionTime.UnixNano()).To(BeNumerically("==", barCondition.LastTransitionTime.UnixNano()))
	})

	It("should emit metrics and events on a transition", func() {
		testObject := TestObject{}
		// Set, but not transitioned
		conditions := testObject.StatusConditions()
		conditions.Set(ctx, status.Condition{Type: "type", Status: metav1.ConditionFalse, Reason: "reason", Message: "message"})
		Expect(GetMetric("operator_status_condition_transition_seconds", map[string]string{
			status.MetricLabelConditionType:   "type",
			status.MetricLabelConditionStatus: string(metav1.ConditionFalse),
		})).To(BeNil())

		// Fake the time to be one second ago
		conditions.Get("type").LastTransitionTime = metav1.Time{Time: conditions.Get("type").LastTransitionTime.Add(-1 * time.Second)}

		// Transition, triggering a metric
		conditions.Set(ctx, status.Condition{Type: "type", Status: metav1.ConditionTrue, Reason: "reason", Message: "message"})
		Expect(GetMetric("operator_status_condition_transition_seconds", map[string]string{
			status.MetricLabelConditionType:   "type",
			status.MetricLabelConditionStatus: string(metav1.ConditionFalse),
		}).GetSummary().GetSampleSum()).To(BeNumerically("~", 1, .01))
		Expect(mockRecorder.Calls()).To(ConsistOf(SatisfyAll(
			HaveField("InvolvedObject", &testObject),
			HaveField("Type", v1.EventTypeNormal),
			HaveField("Reason", "reason"),
			HaveField("Message", "message"),
		)))
	})
})

// GetMetric attempts to find a metric given name and labels
// If no metric is found, the *prometheus.Metric will be nil
func GetMetric(name string, labels map[string]string) *prometheus.Metric {
	family, found := lo.Find(lo.Must(metrics.Registry.Gather()), func(family *prometheus.MetricFamily) bool { return family.GetName() == name })
	if !found {
		return nil
	}
	for _, m := range family.Metric {
		temp := lo.Assign(labels)
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
