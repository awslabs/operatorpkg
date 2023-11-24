package status_test

import (
	"context"
	"time"

	"github.com/awslabs/operatorpkg/status"
	. "github.com/awslabs/operatorpkg/test"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
)

var _ = Describe("Controller", func() {
	var ctx context.Context
	var recorder *record.FakeRecorder
	var controller *status.Controller
	var client client.Client
	BeforeEach(func() {
		recorder = record.NewFakeRecorder(10)
		client = fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
		controller = status.NewController(client, &TestObject{}, recorder)
		ctx = log.IntoContext(context.Background(), ginkgo.GinkgoLogr)
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
		Eventually(recorder.Events).Should(BeEmpty())

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

		Expect(recorder.Events).To(Receive(Equal("Normal Foo Status condition transitioned, Foo: Unknown -> True")))

		// Transition Bar, root condition should also flip
		testObject.StatusConditions().SetTrueWithReason(ConditionTypeBar, "reason", "message")
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

		Expect(recorder.Events).To(Receive(Equal("Normal Bar Status condition transitioned, Bar: Unknown -> True, reason, message")))
		Expect(recorder.Events).To(Receive(Equal("Normal Ready Status condition transitioned, Ready: Unknown -> True")))
	})
})

func conditionLabels(t status.ConditionType, s metav1.ConditionStatus) map[string]string {
	return map[string]string{
		status.MetricLabelConditionType:   string(t),
		status.MetricLabelConditionStatus: string(s),
	}
}
