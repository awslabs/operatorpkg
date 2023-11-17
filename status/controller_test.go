package status_test

import (
	"context"
	"time"

	"github.com/awslabs/operatorpkg/event"
	"github.com/awslabs/operatorpkg/status"
	. "github.com/awslabs/operatorpkg/test"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

var _ = Describe("Controller", func() {

	var ctx context.Context
	var mockRecorder *event.MockRecorder
	var controller *status.Controller
	var client client.Client
	BeforeEach(func() {
		mockRecorder = event.NewMockRecorder()
		client = fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
		controller = status.NewController(client, &TestObject{})
		ctx = context.Background()
		ctx = log.IntoContext(ctx, ginkgo.GinkgoLogr)
		ctx = event.IntoContext(ctx, mockRecorder)
	})

	It("should emit metrics and events on a transition", func() {
		testObject := Object(&TestObject{})
		testObject.StatusConditions() // initialize conditions

		// conditions not set
		ExpectApplied(ctx, client, testObject)
		ExpectToReconcile(ctx, controller, testObject)
		Expect(GetMetric("operator_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transition_seconds")).To(BeNil())
		Expect(mockRecorder.Calls()).To(BeEmpty())

		// Transition Foo
		time.Sleep(time.Second * 1)
		testObject.StatusConditions().SetTrue(ConditionTypeFoo)
		ExpectApplied(ctx, client, testObject)
		ExpectToReconcile(ctx, controller, testObject)

		Expect(GetMetric("operator_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))

		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown)).GetSummary().GetSampleSum()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(mockRecorder.Calls()).To(ConsistOf(SatisfyAll(
			HaveField("InvolvedObject", SatisfyAll(HaveField("Name", testObject.Name), HaveField("Namespace", testObject.Namespace))),
			HaveField("Type", v1.EventTypeWarning),
			HaveField("Reason", string(ConditionTypeFoo)),
			HaveField("Message", ""),
		)))

		// Transition Bar, root condition should also flip
		testObject.StatusConditions().SetTrue(ConditionTypeBar)
		ExpectApplied(ctx, client, testObject)
		ExpectToReconcile(ctx, controller, testObject)

		Expect(GetMetric("operator_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionUnknown)).GetSummary().GetSampleSum()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown)).GetSummary().GetSampleSum()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetSummary().GetSampleSum()).To(BeNumerically(">", 0))

		Expect(mockRecorder.Calls()).To(ConsistOf(
			SatisfyAll(
				HaveField("InvolvedObject", SatisfyAll(HaveField("Name", testObject.Name), HaveField("Namespace", testObject.Namespace))),
				HaveField("Type", v1.EventTypeWarning),
				HaveField("Reason", string(ConditionTypeBar)),
				HaveField("Message", ""),
			),
			SatisfyAll(
				HaveField("InvolvedObject", SatisfyAll(HaveField("Name", testObject.Name), HaveField("Namespace", testObject.Namespace))),
				HaveField("Type", v1.EventTypeWarning),
				HaveField("Reason", string(status.ConditionReady)),
				HaveField("Message", ""),
			),
		))
	})
})

func conditionLabels(t status.ConditionType, s metav1.ConditionStatus) map[string]string {
	return map[string]string{
		status.MetricLabelConditionType:   string(t),
		status.MetricLabelConditionStatus: string(s),
	}
}
