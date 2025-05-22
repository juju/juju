// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"bytes"
	stdtesting "testing"
	time "time"

	"github.com/juju/tc"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/goleak"

	"github.com/juju/juju/internal/testing"
)

type metricsSuite struct {
	baseSuite
}

func TestMetricsSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &metricsSuite{})
}

func (s *metricsSuite) TestMetricsAreCollected(c *tc.C) {
	collector := NewMetricsCollector()

	done := make(chan struct{})
	go func() {
		defer close(done)
		collector.WatermarkInserts.WithLabelValues("foo").Inc()
		collector.WatermarkRetries.WithLabelValues("foo").Inc()
		collector.ChangesRequestDuration.WithLabelValues("foo").Observe(0.42)
		collector.ChangesCount.WithLabelValues("foo").Observe(42.0)
		collector.Subscriptions.WithLabelValues("foo").Set(42)
	}()

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for metrics to be collected")
	}

	expected := bytes.NewBuffer([]byte(`
# HELP juju_db_changestream_count Total number of changes returned by the changestream requests.
# TYPE juju_db_changestream_count histogram
juju_db_changestream_count_bucket{namespace="foo",le="0.005"} 0
juju_db_changestream_count_bucket{namespace="foo",le="0.01"} 0
juju_db_changestream_count_bucket{namespace="foo",le="0.025"} 0
juju_db_changestream_count_bucket{namespace="foo",le="0.05"} 0
juju_db_changestream_count_bucket{namespace="foo",le="0.1"} 0
juju_db_changestream_count_bucket{namespace="foo",le="0.25"} 0
juju_db_changestream_count_bucket{namespace="foo",le="0.5"} 0
juju_db_changestream_count_bucket{namespace="foo",le="1"} 0
juju_db_changestream_count_bucket{namespace="foo",le="2.5"} 0
juju_db_changestream_count_bucket{namespace="foo",le="5"} 0
juju_db_changestream_count_bucket{namespace="foo",le="10"} 0
juju_db_changestream_count_bucket{namespace="foo",le="+Inf"} 1
juju_db_changestream_count_sum{namespace="foo"} 42
juju_db_changestream_count_count{namespace="foo"} 1
# HELP juju_db_subscription_count The total number of subscriptions, labeled per model.
# TYPE juju_db_subscription_count gauge
juju_db_subscription_count{namespace="foo"} 42
# HELP juju_db_watermark_inserts_total Total number of watermark insertions on the changelog witness table.
# TYPE juju_db_watermark_inserts_total counter
juju_db_watermark_inserts_total{namespace="foo"} 1
# HELP juju_db_watermark_retries_total Total number of watermark retries on the changelog witness table.
# TYPE juju_db_watermark_retries_total counter
juju_db_watermark_retries_total{namespace="foo"} 1
		`[1:]))

	err := testutil.CollectAndCompare(
		collector,
		expected,
		"juju_db_watermark_inserts_total",
		"juju_db_watermark_retries_total",
		"juju_db_changestream_count",
		"juju_db_subscription_count",
	)
	if !c.Check(err, tc.ErrorIsNil) {
		c.Logf("\nerror:\n%v", err)
	}
}

func (s *metricsSuite) TestNamespaces(c *tc.C) {
	baseCollector := NewMetricsCollector()
	namespace1Collector := baseCollector.ForNamespace("n1")
	namespace2Collector := baseCollector.ForNamespace("n2")

	done := make(chan struct{})
	go func() {
		defer close(done)

		namespace1Collector.WatermarkRetriesInc()
		namespace2Collector.WatermarkRetriesInc()
	}()

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for metrics to be collected")
	}

	expected := bytes.NewBuffer([]byte(`
# HELP juju_db_watermark_retries_total Total number of watermark retries on the changelog witness table.
# TYPE juju_db_watermark_retries_total counter
juju_db_watermark_retries_total{namespace="n1"} 1
juju_db_watermark_retries_total{namespace="n2"} 1
		`[1:]))

	err := testutil.CollectAndCompare(
		namespace1Collector,
		expected,
		"juju_db_watermark_retries_total",
	)
	if !c.Check(err, tc.ErrorIsNil) {
		c.Logf("\nerror:\n%v", err)
	}
}
