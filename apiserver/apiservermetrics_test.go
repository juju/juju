// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package apiserver_test

import (
	"fmt"
	"regexp"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/version"
)

type apiservermetricsSuite struct {
	testing.IsolationSuite
	collector prometheus.Collector
}

var _ = gc.Suite(&apiservermetricsSuite{})

func (s *apiservermetricsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.collector = apiserver.NewMetricsCollector()
}

func (s *apiservermetricsSuite) TestDescribe(c *gc.C) {
	ch := make(chan *prometheus.Desc)
	go func() {
		defer close(ch)
		s.collector.Describe(ch)
	}()
	var descs []*prometheus.Desc
	for desc := range ch {
		descs = append(descs, desc)
	}
	c.Assert(descs, gc.HasLen, 11)
	c.Assert(descs[0].String(), gc.Matches, `.*fqName: "juju_apiserver_connections_total".*`)
	c.Assert(descs[1].String(), gc.Matches, `.*fqName: "juju_apiserver_connections".*`)
	c.Assert(descs[2].String(), gc.Matches, `.*fqName: "juju_apiserver_active_login_attempts".*`)
	c.Assert(descs[3].String(), gc.Matches, `.*fqName: "juju_apiserver_request_duration_seconds".*`)
	c.Assert(descs[4].String(), gc.Matches, `.*fqName: "juju_apiserver_ping_failure_count".*`)
	c.Assert(descs[5].String(), gc.Matches, `.*fqName: "juju_apiserver_log_write_count".*`)
	c.Assert(descs[6].String(), gc.Matches, `.*fqName: "juju_apiserver_log_read_count".*`)

	c.Assert(descs[7].String(), gc.Matches, `.*fqName: "juju_apiserver_outbound_requests_total".*`)
	c.Assert(descs[8].String(), gc.Matches, `.*fqName: "juju_apiserver_outbound_request_errors_total".*`)
	c.Assert(descs[9].String(), gc.Matches, `.*fqName: "juju_apiserver_outbound_request_duration_seconds".*`)
	build_info_description := descs[10].String()
	c.Check(build_info_description, gc.Matches, `.*fqName: "juju_apiserver_build_info".*`)
	// Ensure that the current version of the Juju controller is one of the const labels on the
	//build_info metric.
	expectedVersionRe := fmt.Sprintf(`.*constLabels:.*version="%s".*`,
		regexp.QuoteMeta(version.Current.String()))
	c.Check(build_info_description, gc.Matches, expectedVersionRe)
}

func (s *apiservermetricsSuite) TestCollect(c *gc.C) {
	ch := make(chan prometheus.Metric)
	go func() {
		defer close(ch)
		s.collector.Collect(ch)
	}()

	var metrics []prometheus.Metric
	for metric := range ch {
		metrics = append(metrics, metric)
	}
	c.Assert(metrics, gc.HasLen, 3)
}

func (s *apiservermetricsSuite) TestLabelNames(c *gc.C) {
	// This is the prometheus label specs.
	labelNameRE := regexp.MustCompile("^[a-zA-Z_][a-zA-Z0-9_]*$")
	testCases := []struct {
		name    string
		labels  []string
		checker gc.Checker
	}{
		{
			name:    "api connections label names",
			labels:  apiserver.MetricAPIConnectionsLabelNames,
			checker: jc.IsTrue,
		},
		{
			name:    "ping failure label names",
			labels:  apiserver.MetricPingFailureLabelNames,
			checker: jc.IsTrue,
		},
		{
			name:    "log failure label names",
			labels:  apiserver.MetricLogLabelNames,
			checker: jc.IsTrue,
		},
		{
			name:    "total requests with status label names",
			labels:  apiserver.MetricTotalRequestsWithStatusLabelNames,
			checker: jc.IsTrue,
		},
		{
			name:    "total requests label names",
			labels:  apiserver.MetricTotalRequestsLabelNames,
			checker: jc.IsTrue,
		},
		{
			name:    "invalid names",
			labels:  []string{"model-uuid"},
			checker: jc.IsFalse,
		},
	}

	for i, testCase := range testCases {
		c.Logf("running test %d", i)
		for k, label := range testCase.labels {
			c.Assert(labelNameRE.MatchString(label), testCase.checker, gc.Commentf("%d %s", k, testCase.name))
		}
	}
}
