// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager_test

import (
	gc "launchpad.net/gocheck"
	"time"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/api/metricsmanager"
	"github.com/juju/juju/testing/factory"
)

type metricsmanagerSuite struct {
	jujutesting.JujuConnSuite

	metricsmanager *metricsmanager.Client
}

var _ = gc.Suite(&metricsmanagerSuite{})

func (s *metricsmanagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.metricsmanager = metricsmanager.NewClient(s.APIState)
	c.Assert(s.metricsmanager, gc.NotNil)
}

func (s *metricsmanagerSuite) TestCleanupOldMetrics(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	oldTime := time.Now().Add(-(time.Hour * 25))
	newTime := time.Now()
	oldMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &oldTime})
	newMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &newTime})
	err := s.metricsmanager.CleanupOldMetrics()
	c.Assert(err, gc.IsNil)
	_, err = s.State.MetricBatch(oldMetric.UUID())
	c.Assert(err, gc.ErrorMatches, "not found")
	_, err = s.State.MetricBatch(newMetric.UUID())
	c.Assert(err, gc.IsNil)
}
