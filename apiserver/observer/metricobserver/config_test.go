// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/observer/metricobserver"
)

type configSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&configSuite{})

func (*configSuite) TestValidateValid(c *gc.C) {
	cfg := metricobserver.Config{
		Clock:                clock.WallClock,
		PrometheusRegisterer: prometheus.NewRegistry(),
	}
	err := cfg.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (*configSuite) TestValidateInvalid(c *gc.C) {
	assertConfigInvalid(c, metricobserver.Config{}, "nil Clock not valid")
	assertConfigInvalid(c, metricobserver.Config{
		Clock: clock.WallClock,
	}, "nil PrometheusRegisterer not valid")
}

func assertConfigInvalid(c *gc.C, cfg metricobserver.Config, expect string) {
	err := cfg.Validate()
	c.Assert(err, gc.ErrorMatches, expect)
}
