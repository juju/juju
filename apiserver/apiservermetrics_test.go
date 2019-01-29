// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package apiserver_test

import (
	"github.com/juju/testing"
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
	c.Assert(descs, gc.HasLen, 9)
	c.Assert(descs[0].String(), gc.Matches, `.*fqName: "juju_apiserver_connections_total".*`)
	c.Assert(descs[1].String(), gc.Matches, `.*fqName: "juju_apiserver_connection_counts".*`)
	c.Assert(descs[2].String(), gc.Matches, `.*fqName: "juju_apiserver_active_login_attempts".*`)
	c.Assert(descs[3].String(), gc.Matches, `.*fqName: "juju_apiserver_request_duration_seconds".*`)
	c.Assert(descs[4].String(), gc.Matches, `.*fqName: "juju_apiserver_ping_failure_count".*`)
	c.Assert(descs[5].String(), gc.Matches, `.*fqName: "juju_apiserver_log_write_count".*`)

	// The following will be removed the future (post 2.6 release)
	c.Assert(descs[6].String(), gc.Matches, `.*fqName: "juju_apiserver_connection_count".*`)
	c.Assert(descs[7].String(), gc.Matches, `.*fqName: "juju_api_requests_total".*`)
	c.Assert(descs[8].String(), gc.Matches, `.*fqName: "juju_api_request_duration_seconds".*`)
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
