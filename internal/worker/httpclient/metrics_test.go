// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpclient

import (
	"bytes"
	"net/url"
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

func (s *metricsSuite) TestRecordError(c *tc.C) {
	collector := NewMetricsCollector()

	u, err := url.Parse("https://api.charmhub.io/v2/charms/refresh")
	c.Assert(err, tc.ErrorIsNil)

	collector.RecordError("POST", u, err)

	expected := bytes.NewBuffer([]byte(`
# HELP juju_http_client_worker_outbound_request_duration_seconds Latency of outbound API requests in seconds.
# TYPE juju_http_client_worker_outbound_request_duration_seconds summary
# HELP juju_http_client_worker_outbound_request_errors_total Total number of http request errors to outbound APIs
# TYPE juju_http_client_worker_outbound_request_errors_total counter
juju_http_client_worker_outbound_request_errors_total{host="api.charmhub.io",status="unknown"} 1
# HELP juju_http_client_worker_outbound_requests_total Total number of http requests to outbound APIs
# TYPE juju_http_client_worker_outbound_requests_total counter
juju_http_client_worker_outbound_requests_total{host="api.charmhub.io",status="unknown"} 1
	`[1:]))

	err = testutil.CollectAndCompare(
		collector,
		expected,
		"juju_http_client_worker_outbound_request_duration_seconds",
		"juju_http_client_worker_outbound_request_errors_total",
		"juju_http_client_worker_outbound_requests_total",
	)
	if !c.Check(err, tc.ErrorIsNil) {
		c.Logf("\nerror:\n%v", err)
	}
}
