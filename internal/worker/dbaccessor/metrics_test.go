// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"bytes"
	time "time"

	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus/testutil"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/testing"
)

type metricsSuite struct {
	baseSuite
}

var _ = gc.Suite(&metricsSuite{})

func (s *metricsSuite) TestMetricsAreCollected(c *gc.C) {
	collector := NewMetricsCollector()

	done := make(chan struct{})
	go func() {
		defer close(done)
		collector.DBDuration.WithLabelValues("foo", "success").Observe(0.1)
		collector.DBErrors.WithLabelValues("foo", "bar").Inc()
		collector.DBSuccess.WithLabelValues("foo").Inc()
		collector.TxnRequests.WithLabelValues("foo").Inc()
		collector.TxnRetries.WithLabelValues("foo").Inc()
	}()

	select {
	case <-done:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for metrics to be collected")
	}

	expected := bytes.NewBuffer([]byte(`
# HELP juju_db_duration_seconds Total time spent in db requests.
# TYPE juju_db_duration_seconds histogram
juju_db_duration_seconds_bucket{namespace="foo",result="success",le="0.005"} 0
juju_db_duration_seconds_bucket{namespace="foo",result="success",le="0.01"} 0
juju_db_duration_seconds_bucket{namespace="foo",result="success",le="0.025"} 0
juju_db_duration_seconds_bucket{namespace="foo",result="success",le="0.05"} 0
juju_db_duration_seconds_bucket{namespace="foo",result="success",le="0.1"} 1
juju_db_duration_seconds_bucket{namespace="foo",result="success",le="0.25"} 1
juju_db_duration_seconds_bucket{namespace="foo",result="success",le="0.5"} 1
juju_db_duration_seconds_bucket{namespace="foo",result="success",le="1"} 1
juju_db_duration_seconds_bucket{namespace="foo",result="success",le="2.5"} 1
juju_db_duration_seconds_bucket{namespace="foo",result="success",le="5"} 1
juju_db_duration_seconds_bucket{namespace="foo",result="success",le="10"} 1
juju_db_duration_seconds_bucket{namespace="foo",result="success",le="+Inf"} 1
juju_db_duration_seconds_sum{namespace="foo",result="success"} 0.1
juju_db_duration_seconds_count{namespace="foo",result="success"} 1
# HELP juju_db_errors_total Total number of db errors.
# TYPE juju_db_errors_total counter
juju_db_errors_total{error="bar",namespace="foo"} 1
# HELP juju_db_success_total Total number of successful db operations.
# TYPE juju_db_success_total counter
juju_db_success_total{namespace="foo"} 1
# HELP juju_db_txn_requests_total Total number of txn requests.
# TYPE juju_db_txn_requests_total counter
juju_db_txn_requests_total{namespace="foo"} 1
# HELP juju_db_txn_retries_total Total number of txn retries.
# TYPE juju_db_txn_retries_total counter
juju_db_txn_retries_total{namespace="foo"} 1
		`[1:]))

	err := testutil.CollectAndCompare(
		collector, expected,
		"juju_db_duration_seconds",
		"juju_db_errors_total",
		"juju_db_success_total",
		"juju_db_txn_requests_total",
		"juju_db_txn_retries_total",
	)
	if !c.Check(err, jc.ErrorIsNil) {
		c.Logf("\nerror:\n%v", err)
	}
}
