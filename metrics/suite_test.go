package metrics_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/awslabs/operatorpkg/metrics"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics")
}

var (
	boolValues = []metrics.Value{{Name: "true"}, {Name: "false"}}
	name       = metrics.Label{Name: "name"}
	exhaustive = metrics.Label{Name: "exhaustive", Values: boolValues, Exhaustive: true}
	documented = metrics.Label{Name: "documented", Values: boolValues}
	other      = metrics.Label{Name: "other", Values: []metrics.Value{{Name: "a"}, {Name: "b"}, {Name: "c"}}, Exhaustive: true}
)

// metric is the subset of the metric interfaces DeletePartialMatch is exercised through.
type metric interface {
	DeletePartialMatch(labels map[string]string)
}

type constructor func(r prometheus.Registerer, labels []metrics.Label) (metric, func(map[string]string), prometheus.Collector)

func gauge(r prometheus.Registerer, labels []metrics.Label) (metric, func(map[string]string), prometheus.Collector) {
	g := metrics.NewPrometheusGauge(r, prometheus.GaugeOpts{Name: "test_gauge", Help: "test"}, labels, metrics.Alpha)
	return g, func(l map[string]string) { g.Set(1, l) }, g.(*metrics.PrometheusGauge).GaugeVec
}

func counter(r prometheus.Registerer, labels []metrics.Label) (metric, func(map[string]string), prometheus.Collector) {
	c := metrics.NewPrometheusCounter(r, prometheus.CounterOpts{Name: "test_counter", Help: "test"}, labels, metrics.Alpha)
	return c, func(l map[string]string) { c.Inc(l) }, c.(*metrics.PrometheusCounter).CounterVec
}

func histogram(r prometheus.Registerer, labels []metrics.Label) (metric, func(map[string]string), prometheus.Collector) {
	h := metrics.NewPrometheusHistogram(r, prometheus.HistogramOpts{Name: "test_histogram", Help: "test"}, labels, metrics.Alpha)
	return h, func(l map[string]string) { h.Observe(1, l) }, h.(*metrics.PrometheusHistogram).HistogramVec
}

func summary(r prometheus.Registerer, labels []metrics.Label) (metric, func(map[string]string), prometheus.Collector) {
	s := metrics.NewPrometheusSummary(r, prometheus.SummaryOpts{Name: "test_summary", Help: "test"}, labels, metrics.Alpha)
	return s, func(l map[string]string) { s.Observe(1, l) }, s.(*metrics.PrometheusSummary).SummaryVec
}

var _ = Describe("DeletePartialMatch", func() {
	DescribeTable("should delete only the matching series",
		func(newMetric constructor, labels []metrics.Label, emitted []map[string]string, partial map[string]string, remaining int) {
			m, emit, collector := newMetric(prometheus.NewRegistry(), labels)
			for _, l := range emitted {
				emit(l)
			}
			Expect(testutil.CollectAndCount(collector)).To(Equal(len(emitted)))
			m.DeletePartialMatch(partial)
			Expect(testutil.CollectAndCount(collector)).To(Equal(remaining))
		},
		func(newMetric constructor, labels []metrics.Label, emitted []map[string]string, partial map[string]string, remaining int) string {
			return fmt.Sprintf("%v deleting %v leaves %d", labels, partial, remaining)
		},
		Entry(nil, gauge, []metrics.Label{name, exhaustive}, []map[string]string{
			{"name": "a", "exhaustive": "true"}, {"name": "a", "exhaustive": "false"}, {"name": "b", "exhaustive": "true"},
		}, map[string]string{"name": "a"}, 1),
		Entry(nil, counter, []metrics.Label{name, exhaustive}, []map[string]string{
			{"name": "a", "exhaustive": "true"}, {"name": "a", "exhaustive": "false"}, {"name": "b", "exhaustive": "true"},
		}, map[string]string{"name": "a"}, 1),
		Entry(nil, histogram, []metrics.Label{name, exhaustive}, []map[string]string{
			{"name": "a", "exhaustive": "true"}, {"name": "a", "exhaustive": "false"}, {"name": "b", "exhaustive": "true"},
		}, map[string]string{"name": "a"}, 1),
		Entry(nil, summary, []metrics.Label{name, exhaustive}, []map[string]string{
			{"name": "a", "exhaustive": "true"}, {"name": "a", "exhaustive": "false"}, {"name": "b", "exhaustive": "true"},
		}, map[string]string{"name": "a"}, 1),
		// every combination of several omitted Exhaustive dimensions
		Entry(nil, gauge, []metrics.Label{name, exhaustive, other}, []map[string]string{
			{"name": "a", "exhaustive": "true", "other": "a"}, {"name": "a", "exhaustive": "false", "other": "c"},
			{"name": "b", "exhaustive": "false", "other": "b"},
		}, map[string]string{"name": "a"}, 1),
		// an Exhaustive dimension may also be given, leaving the others to expand
		Entry(nil, gauge, []metrics.Label{name, exhaustive, other}, []map[string]string{
			{"name": "a", "exhaustive": "true", "other": "a"}, {"name": "a", "exhaustive": "false", "other": "a"},
			{"name": "b", "exhaustive": "true", "other": "b"},
		}, map[string]string{"exhaustive": "true"}, 1),
		// an omitted dimension that is documented but not Exhaustive falls back to scanning
		Entry(nil, gauge, []metrics.Label{name, documented}, []map[string]string{
			{"name": "a", "documented": "true"}, {"name": "a", "documented": "unlisted"}, {"name": "b", "documented": "true"},
		}, map[string]string{"name": "a"}, 1),
		// an omitted unbounded dimension falls back to scanning
		Entry(nil, gauge, []metrics.Label{name, exhaustive}, []map[string]string{
			{"name": "a", "exhaustive": "true"}, {"name": "b", "exhaustive": "true"}, {"name": "b", "exhaustive": "false"},
		}, map[string]string{"exhaustive": "true"}, 1),
		// a dimension the metric doesn't have matches nothing, as with the client's DeletePartialMatch
		Entry(nil, gauge, []metrics.Label{name, exhaustive}, []map[string]string{
			{"name": "a", "exhaustive": "true"}, {"name": "a", "exhaustive": "false"},
		}, map[string]string{"name": "a", "unknown": "x"}, 2),
	)
	DescribeTable("should panic when an Exhaustive dimension is emitted with an unlisted value",
		func(newMetric constructor) {
			_, emit, _ := newMetric(prometheus.NewRegistry(), []metrics.Label{name, exhaustive, documented})
			Expect(func() { emit(map[string]string{"name": "a", "exhaustive": "true", "documented": "unlisted"}) }).ToNot(Panic())
			Expect(func() { emit(map[string]string{"name": "a", "exhaustive": "unlisted", "documented": "true"}) }).To(Panic())
		},
		Entry("gauge", gauge),
		Entry("counter", counter),
		Entry("histogram", histogram),
		Entry("summary", summary),
	)
	It("should panic when a counter is added to with an unlisted value", func() {
		c := metrics.NewPrometheusCounter(prometheus.NewRegistry(), prometheus.CounterOpts{Name: "test_counter", Help: "test"}, []metrics.Label{name, exhaustive}, metrics.Alpha)
		Expect(func() { c.Add(1, map[string]string{"name": "a", "exhaustive": "unlisted"}) }).To(Panic())
	})
})

func BenchmarkDeletePartialMatch(b *testing.B) {
	for _, series := range []int{1_000, 10_000} {
		for _, l := range []metrics.Label{documented, {Name: "documented", Values: boolValues, Exhaustive: true}} {
			b.Run(fmt.Sprintf("series=%d/exhaustive=%t", series, l.Exhaustive), func(b *testing.B) {
				g := metrics.NewPrometheusGauge(prometheus.NewRegistry(), prometheus.GaugeOpts{Name: "bench", Help: "bench"}, []metrics.Label{name, l}, metrics.Alpha)
				for i := range series {
					g.Set(1, map[string]string{"name": fmt.Sprint(i), "documented": "true"})
				}
				b.ResetTimer()
				for i := range b.N {
					n := fmt.Sprint(i % series)
					g.DeletePartialMatch(map[string]string{"name": n})
					g.Set(1, map[string]string{"name": n, "documented": "true"})
				}
			})
		}
	}
}
