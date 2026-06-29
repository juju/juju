// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"bytes"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/goleak"
)

type metricsSuite struct{}

func TestMetricsSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &metricsSuite{})
}

func (s *metricsSuite) TestMetricsAreCollected(c *tc.C) {
	collector := NewMetricsCollector()

	collector.recordRequest("/loki-endpoint", "POST", 200, 0.1)
	collector.recordRequest("/loki-endpoint", "POST", 500, 0.2)

	expected := bytes.NewBuffer([]byte(`
# HELP juju_control_socket_request_duration_seconds Control socket request duration in seconds.
# TYPE juju_control_socket_request_duration_seconds histogram
juju_control_socket_request_duration_seconds_bucket{endpoint="/loki-endpoint",method="POST",le="0.005"} 0
juju_control_socket_request_duration_seconds_bucket{endpoint="/loki-endpoint",method="POST",le="0.01"} 0
juju_control_socket_request_duration_seconds_bucket{endpoint="/loki-endpoint",method="POST",le="0.025"} 0
juju_control_socket_request_duration_seconds_bucket{endpoint="/loki-endpoint",method="POST",le="0.05"} 0
juju_control_socket_request_duration_seconds_bucket{endpoint="/loki-endpoint",method="POST",le="0.1"} 1
juju_control_socket_request_duration_seconds_bucket{endpoint="/loki-endpoint",method="POST",le="0.25"} 2
juju_control_socket_request_duration_seconds_bucket{endpoint="/loki-endpoint",method="POST",le="0.5"} 2
juju_control_socket_request_duration_seconds_bucket{endpoint="/loki-endpoint",method="POST",le="1"} 2
juju_control_socket_request_duration_seconds_bucket{endpoint="/loki-endpoint",method="POST",le="2.5"} 2
juju_control_socket_request_duration_seconds_bucket{endpoint="/loki-endpoint",method="POST",le="5"} 2
juju_control_socket_request_duration_seconds_bucket{endpoint="/loki-endpoint",method="POST",le="10"} 2
juju_control_socket_request_duration_seconds_bucket{endpoint="/loki-endpoint",method="POST",le="+Inf"} 2
juju_control_socket_request_duration_seconds_sum{endpoint="/loki-endpoint",method="POST"} 0.30000000000000004
juju_control_socket_request_duration_seconds_count{endpoint="/loki-endpoint",method="POST"} 2
# HELP juju_control_socket_request_errors_total Total number of failed control socket requests.
# TYPE juju_control_socket_request_errors_total counter
juju_control_socket_request_errors_total{endpoint="/loki-endpoint",method="POST",status="500"} 1
# HELP juju_control_socket_requests_total Total number of control socket requests.
# TYPE juju_control_socket_requests_total counter
juju_control_socket_requests_total{endpoint="/loki-endpoint",method="POST",status="200"} 1
juju_control_socket_requests_total{endpoint="/loki-endpoint",method="POST",status="500"} 1
		`[1:]))

	err := testutil.CollectAndCompare(
		collector, expected,
		"juju_control_socket_request_duration_seconds",
		"juju_control_socket_request_errors_total",
		"juju_control_socket_requests_total",
	)
	if !c.Check(err, tc.ErrorIsNil) {
		c.Logf("\nerror:\n%v", err)
	}
}
