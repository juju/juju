// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package apiserver_test

import (
	"regexp"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
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
	c.Assert(descs, gc.HasLen, 7)
	c.Assert(descs[0].String(), gc.Matches, `.*fqName: "juju_apiserver_connections_total".*`)
	c.Assert(descs[1].String(), gc.Matches, `.*fqName: "juju_apiserver_connections".*`)
	c.Assert(descs[2].String(), gc.Matches, `.*fqName: "juju_apiserver_active_login_attempts".*`)
	c.Assert(descs[3].String(), gc.Matches, `.*fqName: "juju_apiserver_request_duration_seconds".*`)
	c.Assert(descs[4].String(), gc.Matches, `.*fqName: "juju_apiserver_ping_failure_count".*`)
	c.Assert(descs[5].String(), gc.Matches, `.*fqName: "juju_apiserver_log_write_count".*`)
	c.Assert(descs[6].String(), gc.Matches, `.*fqName: "juju_apiserver_log_read_count".*`)
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
	c.Assert(metrics, gc.HasLen, 2)
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
