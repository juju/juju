// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api/metricsmanager"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type metricsManagerSuite struct {
	jujutesting.JujuConnSuite

	manager *metricsmanager.Client
}

var _ = gc.Suite(&metricsManagerSuite{})

func (s *metricsManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.manager = metricsmanager.NewClient(s.APIState)
	c.Assert(s.manager, gc.NotNil)
}

func (s *metricsManagerSuite) TestCleanupOldMetrics(c *gc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{SetCharmURL: true})
	oldTime := time.Now().Add(-(time.Hour * 25))
	newTime := time.Now()
	oldMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &oldTime})
	newMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &newTime})
	err := s.manager.CleanupOldMetrics()
	c.Assert(err, gc.IsNil)
	_, err = s.State.MetricBatch(oldMetric.UUID())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.State.MetricBatch(newMetric.UUID())
	c.Assert(err, gc.IsNil)
}
