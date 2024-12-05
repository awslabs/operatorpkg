package status_test

import (
	"context"
	"sync"
	"time"

	pmetrics "github.com/awslabs/operatorpkg/metrics"
	"github.com/awslabs/operatorpkg/object"
	"github.com/awslabs/operatorpkg/status"
	"github.com/awslabs/operatorpkg/test"
	. "github.com/awslabs/operatorpkg/test/expectations"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var ctx context.Context
var recorder *record.FakeRecorder
var kubeClient client.Client
var registry = metrics.Registry

var _ = BeforeEach(func() {
	metrics.Registry = registry // reset the registry to handle cases where the registry is overridden
})

var _ = AfterEach(func() {
	status.ConditionDuration.Reset()
	status.ConditionCount.Reset()
	status.ConditionCurrentStatusSeconds.Reset()
	status.ConditionTransitionsTotal.Reset()
	status.TerminationCurrentTimeSeconds.Reset()
	status.TerminationDuration.Reset()
})

var _ = Describe("Controller", func() {
	var ctx context.Context
	var recorder *record.FakeRecorder
	var controller *status.Controller[*test.CustomObject]
	var kubeClient client.Client
	BeforeEach(func() {
		recorder = record.NewFakeRecorder(10)
		kubeClient = fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
		ctx = log.IntoContext(context.Background(), GinkgoLogr)
		controller = status.NewController[*test.CustomObject](kubeClient, recorder, status.EmitDeprecatedMetrics)
	})
	AfterEach(func() {
		registry.Unregister(controller.ConditionDuration.(*pmetrics.PrometheusHistogram).HistogramVec)
		registry.Unregister(controller.ConditionCount.(*pmetrics.PrometheusGauge).GaugeVec)
		registry.Unregister(controller.ConditionCurrentStatusSeconds.(*pmetrics.PrometheusGauge).GaugeVec)
		registry.Unregister(controller.ConditionTransitionsTotal.(*pmetrics.PrometheusCounter).CounterVec)
		registry.Unregister(controller.TerminationCurrentTimeSeconds.(*pmetrics.PrometheusGauge).GaugeVec)
		registry.Unregister(controller.TerminationDuration.(*pmetrics.PrometheusHistogram).HistogramVec)
	})
	It("should emit termination metrics when deletion timestamp is set", func() {
		testObject := test.Object(&test.CustomObject{})
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectDeletionTimestampSet(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)
		metric := GetMetric("operator_termination_current_time_seconds", map[string]string{status.MetricLabelName: testObject.Name})
		Expect(metric).ToNot(BeNil())
		Expect(metric.GetGauge().GetValue()).To(BeNumerically(">", 0))
		metric = GetMetric("operator_customobject_termination_current_time_seconds", map[string]string{status.MetricLabelName: testObject.Name})
		Expect(metric).ToNot(BeNil())
		Expect(metric.GetGauge().GetValue()).To(BeNumerically(">", 0))

		// Patch the finalizer
		mergeFrom := client.MergeFrom(testObject.DeepCopyObject().(client.Object))
		testObject.SetFinalizers([]string{})
		Expect(client.IgnoreNotFound(kubeClient.Patch(ctx, testObject, mergeFrom))).To(Succeed())
		ExpectReconciled(ctx, controller, testObject)
		Expect(GetMetric("operator_termination_current_time_seconds", map[string]string{status.MetricLabelName: testObject.Name})).To(BeNil())
		Expect(GetMetric("operator_customobject_termination_current_time_seconds", map[string]string{status.MetricLabelName: testObject.Name})).To(BeNil())
		metric = GetMetric("operator_termination_duration_seconds", map[string]string{})
		Expect(metric).ToNot(BeNil())
		Expect(metric.GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		metric = GetMetric("operator_customobject_termination_duration_seconds", map[string]string{})
		Expect(metric).ToNot(BeNil())
		Expect(metric.GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
	})
	It("should emit metrics and events on a transition", func() {
		testObject := test.Object(&test.CustomObject{})
		gvk := object.GVK(testObject)
		testObject.StatusConditions() // initialize conditions

		// conditions not set
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)

		// Ready Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		// Foo Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		// Bar Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_status_condition_transition_seconds")).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total")).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_transition_seconds")).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total")).To(BeNil())

		Eventually(recorder.Events).Should(BeEmpty())

		// Transition Foo
		time.Sleep(time.Second * 1)
		testObject.StatusConditions().SetTrue(test.ConditionTypeFoo)
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)
		ExpectStatusConditions(ctx, kubeClient, FastTimeout, testObject, status.Condition{Type: test.ConditionTypeFoo, Status: metav1.ConditionTrue})

		// Ready Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		// Foo Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).ToNot(BeZero())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).ToNot(BeZero())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		// Bar Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(recorder.Events).To(Receive(Equal("Normal Foo Status condition transitioned, Type: Foo, Status: Unknown -> True, Reason: Foo")))

		// Transition Bar, root condition should also flip
		testObject.StatusConditions().SetTrueWithReason(test.ConditionTypeBar, "reason", "message")
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)
		ExpectStatusConditions(ctx, kubeClient, FastTimeout, testObject, status.Condition{Type: test.ConditionTypeBar, Status: metav1.ConditionTrue, Reason: "reason", Message: "message"})

		// Ready Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())

		// Foo Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		// Bar Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		// Ready Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())

		// Foo Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		// Bar Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))

		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))

		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(status.ConditionReady, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(recorder.Events).To(Receive(Equal("Normal Bar Status condition transitioned, Type: Bar, Status: Unknown -> True, Reason: reason, Message: message")))
		Expect(recorder.Events).To(Receive(Equal("Normal Ready Status condition transitioned, Type: Ready, Status: Unknown -> True, Reason: Ready")))

		// Delete the object, state should clear
		ExpectDeleted(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)

		// Ready Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())

		// Foo Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		// Bar Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
	})
	It("should emit transition total metrics for abnormal conditions", func() {
		testObject := test.Object(&test.CustomObject{})
		gvk := object.GVK(testObject)
		testObject.StatusConditions() // initialize conditions

		// conditions not set
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)

		// set the bar condition and transition it to true
		testObject.StatusConditions().SetTrue(ConditionTypeBar)

		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)
		ExpectStatusConditions(ctx, kubeClient, FastTimeout, testObject, status.Condition{Type: test.ConditionTypeBar, Status: metav1.ConditionTrue, Reason: test.ConditionTypeBar, Message: ""})

		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, test.ConditionTypeBar, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, test.ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, test.ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(test.ConditionTypeBar, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(test.ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(test.ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		// set the bar condition and transition it to false
		testObject.StatusConditions().SetFalse(test.ConditionTypeBar, "reason", "message")

		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)
		ExpectStatusConditions(ctx, kubeClient, FastTimeout, testObject, status.Condition{Type: test.ConditionTypeBar, Status: metav1.ConditionFalse, Reason: "reason", Message: "message"})

		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, test.ConditionTypeBar, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, test.ConditionTypeBar, metav1.ConditionFalse)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, test.ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(test.ConditionTypeBar, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(test.ConditionTypeBar, metav1.ConditionFalse)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(test.ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		// clear the condition and don't expect the metrics to change
		_ = testObject.StatusConditions().Clear(test.ConditionTypeBar)

		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)

		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, test.ConditionTypeBar, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, test.ConditionTypeBar, metav1.ConditionFalse)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, test.ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(test.ConditionTypeBar, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(test.ConditionTypeBar, metav1.ConditionFalse)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(test.ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
	})
	It("should not race when reconciling status conditions simultaneously", func() {
		var objs []*test.CustomObject
		for range 100 {
			testObject := test.Object(&test.CustomObject{})
			testObject.StatusConditions() // initialize conditions
			// conditions not set
			ExpectApplied(ctx, kubeClient, testObject)
			objs = append(objs, testObject)
		}

		// Run 100 object reconciles at once to attempt to trigger a data raceg
		var wg sync.WaitGroup
		for _, obj := range objs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()

				ExpectReconciled(ctx, controller, obj)
			}()
		}

		for _, obj := range objs {
			// set the baz condition and transition it to true
			obj.StatusConditions().SetTrue(test.ConditionTypeBar)
			ExpectApplied(ctx, kubeClient, obj)
		}

		// Run 100 object reconciles at once to attempt to trigger a data race
		for _, obj := range objs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()

				ExpectReconciled(ctx, controller, obj)
			}()
		}
	})
	It("should set LastTransitionTime for status conditions on initialization to CreationTimestamp", func() {
		testObject := test.Object(&test.CustomObject{})
		testObject.StatusConditions() // initialize conditions after applying and setting CreationTimestamp

		Expect(testObject.StatusConditions().Get(test.ConditionTypeFoo).LastTransitionTime.Time).To(Equal(testObject.GetCreationTimestamp().Time))
		Expect(testObject.StatusConditions().Get(test.ConditionTypeBar).LastTransitionTime.Time).To(Equal(testObject.GetCreationTimestamp().Time))
		Expect(testObject.StatusConditions().Get(status.ConditionReady).LastTransitionTime.Time).To(Equal(testObject.GetCreationTimestamp().Time))
	})
	It("should consider status conditions that aren't set as unknown", func() {
		// This mimics an object creation
		testObject := test.Object(&test.CustomObject{})
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)

		// Then the status conditions gets initialized and a condition is set to True
		testObject.StatusConditions().SetTrue(test.ConditionTypeFoo)
		testObject.SetCreationTimestamp(metav1.Time{Time: time.Now().Add(time.Hour)})
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)

		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(status.ConditionReady, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(status.ConditionReady, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(status.ConditionReady, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
	})
	It("should add labels to metrics when using WithLabels", func() {
		metrics.Registry = prometheus.NewRegistry()
		controller = status.NewController[*test.CustomObject](kubeClient, recorder, status.WithLabels("operator.pkg/key1", "operator.pkg/key2"))
		testObject := test.Object(&test.CustomObject{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"operator.pkg/key1": "value1",
					"operator.pkg/key2": "value2",
				},
			},
		})
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)

		// Transition Foo
		time.Sleep(time.Second * 1)
		testObject.StatusConditions().SetTrue(test.ConditionTypeFoo)
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)
		ExpectStatusConditions(ctx, kubeClient, FastTimeout, testObject, status.Condition{Type: test.ConditionTypeFoo, Status: metav1.ConditionTrue})

		// Ready Condition
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(status.ConditionReady, metav1.ConditionTrue), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(status.ConditionReady, metav1.ConditionFalse), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(status.ConditionReady, metav1.ConditionUnknown), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"})).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(status.ConditionReady, metav1.ConditionTrue)), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"})).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(status.ConditionReady, metav1.ConditionFalse)), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"})).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(status.ConditionReady, metav1.ConditionUnknown), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"})).GetGauge().GetValue()).ToNot(BeZero())

		// Foo Condition
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(ConditionTypeFoo, metav1.ConditionTrue), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"})).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(ConditionTypeFoo, metav1.ConditionFalse), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(ConditionTypeFoo, metav1.ConditionTrue), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).ToNot(BeZero())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(ConditionTypeFoo, metav1.ConditionFalse), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())

		// Bar Condition
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(ConditionTypeBar, metav1.ConditionTrue), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(ConditionTypeBar, metav1.ConditionFalse), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(ConditionTypeBar, metav1.ConditionUnknown), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"})).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(ConditionTypeBar, metav1.ConditionTrue), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(ConditionTypeBar, metav1.ConditionFalse), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(ConditionTypeBar, metav1.ConditionUnknown), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"})).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", lo.Assign(conditionLabels(status.ConditionReady, metav1.ConditionTrue), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", lo.Assign(conditionLabels(status.ConditionReady, metav1.ConditionFalse), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", lo.Assign(conditionLabels(status.ConditionReady, metav1.ConditionUnknown), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", lo.Assign(conditionLabels(ConditionTypeFoo, metav1.ConditionTrue), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", lo.Assign(conditionLabels(ConditionTypeFoo, metav1.ConditionFalse), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", lo.Assign(conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"})).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", lo.Assign(conditionLabels(ConditionTypeBar, metav1.ConditionTrue), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", lo.Assign(conditionLabels(ConditionTypeBar, metav1.ConditionFalse), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transition_seconds", lo.Assign(conditionLabels(ConditionTypeBar, metav1.ConditionUnknown), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())

		Expect(GetMetric("operator_customobject_status_condition_transitions_total", lo.Assign(conditionLabels(status.ConditionReady, metav1.ConditionTrue), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", lo.Assign(conditionLabels(status.ConditionReady, metav1.ConditionFalse), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", lo.Assign(conditionLabels(status.ConditionReady, metav1.ConditionUnknown), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", lo.Assign(conditionLabels(ConditionTypeFoo, metav1.ConditionTrue), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"})).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", lo.Assign(conditionLabels(ConditionTypeFoo, metav1.ConditionFalse), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", lo.Assign(conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", lo.Assign(conditionLabels(ConditionTypeBar, metav1.ConditionTrue), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", lo.Assign(conditionLabels(ConditionTypeBar, metav1.ConditionFalse), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_transitions_total", lo.Assign(conditionLabels(ConditionTypeBar, metav1.ConditionUnknown), map[string]string{"operator_pkg_key1": "value1", "operator_pkg_key2": "value2"}))).To(BeNil())
	})
	It("should ensure that we don't leak metrics when changing reason with the same status", func() {
		testObject := test.Object(&test.CustomObject{})
		testObject.StatusConditions() // initialize conditions

		// conditions not set (this means that we should see that the condition is in an "Unknown" state)
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)

		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionTrue), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionFalse), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionUnknown), map[string]string{"reason": "AwaitingReconciliation"})).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionTrue), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionFalse), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionUnknown), map[string]string{"reason": "AwaitingReconciliation"})).GetGauge().GetValue()).ToNot(BeZero())

		// Set Foo explicitly to Unknown (this shouldn't change the reason at all)
		testObject.StatusConditions().SetUnknown(test.ConditionTypeFoo)
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)
		ExpectStatusConditions(ctx, kubeClient, FastTimeout, testObject, status.Condition{Type: test.ConditionTypeFoo, Status: metav1.ConditionUnknown})

		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionTrue), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionFalse), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionUnknown), map[string]string{"reason": "AwaitingReconciliation"})).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionTrue), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionFalse), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionUnknown), map[string]string{"reason": "AwaitingReconciliation"})).GetGauge().GetValue()).ToNot(BeZero())

		// Set Foo to Unknown but with a custom reason (we should change the reason abut not leak metrics)
		testObject.StatusConditions().SetUnknownWithReason(test.ConditionTypeFoo, "CustomReason", "custom message")
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, controller, testObject)
		ExpectStatusConditions(ctx, kubeClient, FastTimeout, testObject, status.Condition{Type: test.ConditionTypeFoo, Status: metav1.ConditionUnknown})

		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionTrue), map[string]string{"reason": "CustomReason"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionFalse), map[string]string{"reason": "CustomReason"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionUnknown), map[string]string{"reason": "CustomReason"})).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionTrue), map[string]string{"reason": "CustomReason"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionFalse), map[string]string{"reason": "CustomReason"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionUnknown), map[string]string{"reason": "CustomReason"})).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionTrue), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionFalse), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_count", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionUnknown), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionTrue), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionFalse), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
		Expect(GetMetric("operator_customobject_status_condition_current_status_seconds", lo.Assign(conditionLabels(test.ConditionTypeFoo, metav1.ConditionUnknown), map[string]string{"reason": "AwaitingReconciliation"}))).To(BeNil())
	})
})

var _ = Describe("Generic Controller", func() {
	var genericController *status.GenericObjectController[*TestGenericObject]
	BeforeEach(func() {
		recorder = record.NewFakeRecorder(10)
		kubeClient = fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
		ctx = log.IntoContext(context.Background(), GinkgoLogr)
		genericController = status.NewGenericObjectController[*TestGenericObject](kubeClient, recorder, status.EmitDeprecatedMetrics)
	})
	AfterEach(func() {
		registry.Unregister(genericController.ConditionDuration.(*pmetrics.PrometheusHistogram).HistogramVec)
		registry.Unregister(genericController.ConditionCount.(*pmetrics.PrometheusGauge).GaugeVec)
		registry.Unregister(genericController.ConditionCurrentStatusSeconds.(*pmetrics.PrometheusGauge).GaugeVec)
		registry.Unregister(genericController.ConditionTransitionsTotal.(*pmetrics.PrometheusCounter).CounterVec)
		registry.Unregister(genericController.TerminationCurrentTimeSeconds.(*pmetrics.PrometheusGauge).GaugeVec)
		registry.Unregister(genericController.TerminationDuration.(*pmetrics.PrometheusHistogram).HistogramVec)
	})
	It("should emit termination metrics when deletion timestamp is set", func() {
		testObject := test.Object(&TestGenericObject{})
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectDeletionTimestampSet(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, genericController, testObject)
		metric := GetMetric("operator_termination_current_time_seconds", map[string]string{status.MetricLabelName: testObject.Name})
		Expect(metric).ToNot(BeNil())
		Expect(metric.GetGauge().GetValue()).To(BeNumerically(">", 0))
		metric = GetMetric("operator_testgenericobject_termination_current_time_seconds", map[string]string{status.MetricLabelName: testObject.Name})
		Expect(metric).ToNot(BeNil())
		Expect(metric.GetGauge().GetValue()).To(BeNumerically(">", 0))

		// Patch the finalizer
		mergeFrom := client.MergeFrom(testObject.DeepCopyObject().(client.Object))
		testObject.SetFinalizers([]string{})
		Expect(client.IgnoreNotFound(kubeClient.Patch(ctx, testObject, mergeFrom))).To(Succeed())
		ExpectReconciled(ctx, genericController, testObject)
		Expect(GetMetric("operator_termination_current_time_seconds", map[string]string{status.MetricLabelName: testObject.Name})).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_termination_current_time_seconds", map[string]string{status.MetricLabelName: testObject.Name})).To(BeNil())
		metric = GetMetric("operator_termination_duration_seconds", map[string]string{})
		Expect(metric).ToNot(BeNil())
		Expect(metric.GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		metric = GetMetric("operator_testgenericobject_termination_duration_seconds", map[string]string{})
		Expect(metric).ToNot(BeNil())
		Expect(metric.GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
	})
	It("should emit metrics and events on a transition", func() {
		testObject := test.Object(&TestGenericObject{})
		gvk := object.GVK(testObject)
		testObject.Status = TestGenericStatus{
			Conditions: []metav1.Condition{
				{
					Type:   ConditionTypeFoo,
					Status: metav1.ConditionUnknown,
				},
				{
					Type:   ConditionTypeBar,
					Status: metav1.ConditionUnknown,
				},
			},
		}

		// conditions not set
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, genericController, testObject)

		// Foo Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		// Bar Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_status_condition_transition_seconds", map[string]string{pmetrics.LabelKind: gvk.Kind})).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", map[string]string{pmetrics.LabelKind: gvk.Kind})).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", map[string]string{pmetrics.LabelKind: gvk.Kind})).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", map[string]string{pmetrics.LabelKind: gvk.Kind})).To(BeNil())

		Eventually(recorder.Events).Should(BeEmpty())

		// Transition Foo
		time.Sleep(time.Second * 1)
		testObject.Status = TestGenericStatus{
			Conditions: []metav1.Condition{
				{
					Type:   ConditionTypeFoo,
					Status: metav1.ConditionTrue,
					Reason: ConditionTypeFoo,
				},
				{
					Type:   ConditionTypeBar,
					Status: metav1.ConditionUnknown,
				},
			},
		}
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, genericController, testObject)

		// Foo Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).ToNot(BeZero())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).ToNot(BeZero())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		// Bar Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetGauge().GetValue()).ToNot(BeZero())

		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(recorder.Events).To(Receive(Equal("Normal Foo Status condition transitioned, Type: Foo, Status: Unknown -> True, Reason: Foo")))

		// Transition Bar, root condition should also flip
		testObject.Status = TestGenericStatus{
			Conditions: []metav1.Condition{
				{
					Type:   ConditionTypeFoo,
					Status: metav1.ConditionTrue,
					Reason: ConditionTypeFoo,
				},
				{
					Type:    ConditionTypeBar,
					Status:  metav1.ConditionTrue,
					Reason:  "reason",
					Message: "message",
				},
			},
		}
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, genericController, testObject)

		// Foo Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		// Bar Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		// Foo Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		// Bar Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue)).GetGauge().GetValue()).ToNot(BeZero())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transition_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))

		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))
		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transition_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown)).GetHistogram().GetSampleCount()).To(BeNumerically(">", 0))

		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(recorder.Events).To(Receive(Equal("Normal Bar Status condition transitioned, Type: Bar, Status: Unknown -> True, Reason: reason, Message: message")))

		// Delete the object, state should clear
		ExpectDeleted(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, genericController, testObject)

		// Foo Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeFoo, metav1.ConditionUnknown))).To(BeNil())

		// Bar Condition
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_count", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_current_status_seconds", conditionLabelsWithGroupKind(gvk, ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_count", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionTrue))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_current_status_seconds", conditionLabels(ConditionTypeBar, metav1.ConditionUnknown))).To(BeNil())
	})
	It("should emit transition total metrics for abnormal conditions", func() {
		testObject := test.Object(&TestGenericObject{})
		gvk := object.GVK(testObject)
		testObject.Status = TestGenericStatus{
			Conditions: []metav1.Condition{
				{
					Type:   ConditionTypeFoo,
					Status: metav1.ConditionUnknown,
				},
				{
					Type:   ConditionTypeBaz,
					Status: metav1.ConditionUnknown,
				},
			},
		}

		// conditions not set
		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, genericController, testObject)

		// set the baz condition and transition it to true
		testObject.Status = TestGenericStatus{
			Conditions: []metav1.Condition{
				{
					Type:   ConditionTypeFoo,
					Status: metav1.ConditionUnknown,
				},
				{
					Type:   ConditionTypeBaz,
					Status: metav1.ConditionTrue,
				},
			},
		}

		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, genericController, testObject)

		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBaz, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBaz, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBaz, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBaz, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBaz, metav1.ConditionFalse))).To(BeNil())
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBaz, metav1.ConditionUnknown))).To(BeNil())

		// set the bar condition and transition it to false
		testObject.Status = TestGenericStatus{
			Conditions: []metav1.Condition{
				{
					Type:   ConditionTypeFoo,
					Status: metav1.ConditionTrue,
					Reason: ConditionTypeFoo,
				},
				{
					Type:    ConditionTypeBaz,
					Status:  metav1.ConditionFalse,
					Reason:  "reason",
					Message: "message",
				},
			},
		}

		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, genericController, testObject)

		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBaz, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBaz, metav1.ConditionFalse)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBaz, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBaz, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBaz, metav1.ConditionFalse)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBaz, metav1.ConditionUnknown))).To(BeNil())

		// clear the condition and don't expect the metrics to change
		testObject.Status = TestGenericStatus{
			Conditions: []metav1.Condition{
				{
					Type:   ConditionTypeFoo,
					Status: metav1.ConditionTrue,
					Reason: ConditionTypeFoo,
				},
			},
		}

		ExpectApplied(ctx, kubeClient, testObject)
		ExpectReconciled(ctx, genericController, testObject)

		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBaz, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBaz, metav1.ConditionFalse)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_status_condition_transitions_total", conditionLabelsWithGroupKind(gvk, ConditionTypeBaz, metav1.ConditionUnknown))).To(BeNil())

		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBaz, metav1.ConditionTrue)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBaz, metav1.ConditionFalse)).GetCounter().GetValue()).To(BeEquivalentTo(1))
		Expect(GetMetric("operator_testgenericobject_status_condition_transitions_total", conditionLabels(ConditionTypeBaz, metav1.ConditionUnknown))).To(BeNil())
	})
	It("should not race when reconciling status conditions simultaneously", func() {
		var objs []*TestGenericObject
		for range 100 {
			testObject := test.Object(&TestGenericObject{})
			testObject.Status = TestGenericStatus{
				Conditions: []metav1.Condition{
					{
						Type:   ConditionTypeFoo,
						Status: metav1.ConditionUnknown,
					},
					{
						Type:   ConditionTypeBaz,
						Status: metav1.ConditionUnknown,
					},
				},
			}
			// conditions not set
			ExpectApplied(ctx, kubeClient, testObject)
			objs = append(objs, testObject)
		}

		// Run 100 object reconciles at once to attempt to trigger a data raceg
		var wg sync.WaitGroup
		for _, obj := range objs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()

				ExpectReconciled(ctx, genericController, obj)
			}()
		}

		for _, obj := range objs {
			// set the baz condition and transition it to true
			obj.Status = TestGenericStatus{
				Conditions: []metav1.Condition{
					{
						Type:   ConditionTypeFoo,
						Status: metav1.ConditionUnknown,
					},
					{
						Type:   ConditionTypeBaz,
						Status: metav1.ConditionTrue,
					},
				},
			}
			ExpectApplied(ctx, kubeClient, obj)
		}

		// Run 100 object reconciles at once to attempt to trigger a data race
		for _, obj := range objs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer GinkgoRecover()

				ExpectReconciled(ctx, genericController, obj)
			}()
		}
	})
})

func conditionLabelsWithGroupKind(gvk schema.GroupVersionKind, t status.ConditionType, s metav1.ConditionStatus) map[string]string {
	return map[string]string{
		pmetrics.LabelGroup:               gvk.Group,
		pmetrics.LabelKind:                gvk.Kind,
		pmetrics.LabelType:                string(t),
		status.MetricLabelConditionStatus: string(s),
	}
}

func conditionLabels(t status.ConditionType, s metav1.ConditionStatus) map[string]string {
	return map[string]string{
		pmetrics.LabelType:                string(t),
		status.MetricLabelConditionStatus: string(s),
	}
}
