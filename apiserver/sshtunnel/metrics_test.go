// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunnel

import (
	"bytes"
	"testing"

	"github.com/juju/tc"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/juju/juju/internal/testhelpers"
)

type metricsSuite struct{}

func TestMetricsSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &metricsSuite{})
	})
}

func (s *metricsSuite) TestMetricsAreCollected(c *tc.C) {
	collector := NewMetricsCollector()

	collector.IncConnectionCount("relay")
	collector.IncConnectionCount("tunnel")
	collector.IncConnectionCount("tunnel")
	collector.DecConnectionCount("tunnel")

	expected := bytes.NewBuffer([]byte(`
# HELP juju_sshtunnel_connection_count The number of active SSH tunnel or relay upgrade connections.
# TYPE juju_sshtunnel_connection_count gauge
juju_sshtunnel_connection_count{endpoint="relay"} 1
juju_sshtunnel_connection_count{endpoint="tunnel"} 1
`[1:]))

	err := testutil.CollectAndCompare(
		collector, expected,
		"juju_sshtunnel_connection_count",
	)
	if !c.Check(err, tc.ErrorIsNil) {
		c.Logf("\nerror:\n%v", err)
	}
}
