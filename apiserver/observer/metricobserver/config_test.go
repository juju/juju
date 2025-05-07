// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricobserver_test

import (
	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/observer/metricobserver"
	"github.com/juju/juju/apiserver/observer/metricobserver/mocks"
)

type configSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&configSuite{})

func (*configSuite) TestValidateValid(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	metricsCollector := mocks.NewMockMetricsCollector(ctrl)

	cfg := metricobserver.Config{
		Clock:            clock.WallClock,
		MetricsCollector: metricsCollector,
	}
	err := cfg.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (*configSuite) TestValidateInvalid(c *tc.C) {
	assertConfigInvalid(c, metricobserver.Config{}, "nil Clock not valid")
	assertConfigInvalid(c, metricobserver.Config{
		Clock: clock.WallClock,
	}, "nil MetricsCollector not valid")
}

func assertConfigInvalid(c *tc.C, cfg metricobserver.Config, expect string) {
	err := cfg.Validate()
	c.Assert(err, tc.ErrorMatches, expect)
}
