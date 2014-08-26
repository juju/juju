// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsmanager_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/apiserver/metricsmanager"
	apiservertesting "github.com/juju/juju/state/apiserver/testing"
	"github.com/juju/juju/testing/factory"
)

type metricsManagerSuite struct {
	jujutesting.JujuConnSuite

	metricsmanager *metricsmanager.MetricsManagerAPI
	authorizer     apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&metricsManagerSuite{})

func (s *metricsManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	user, err := s.State.User("admin")
	c.Assert(err, gc.IsNil)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: user.Tag(),
	}
	s.metricsmanager, err = metricsmanager.NewMetricsManagerAPI(s.State, nil, s.authorizer)
	c.Assert(err, gc.IsNil)
}

func (s *metricsManagerSuite) TestCleanupOldMetrics(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	oldTime := time.Now().Add(-(time.Hour * 25))
	newTime := time.Now()
	oldMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &oldTime})
	newMetric := s.Factory.MakeMetric(c, &factory.MetricParams{Unit: unit, Sent: true, Time: &newTime})
	_, err := s.metricsmanager.CleanupOldMetrics()
	c.Assert(err, gc.IsNil)
	_, err = s.State.MetricBatch(oldMetric.UUID())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	_, err = s.State.MetricBatch(newMetric.UUID())
	c.Assert(err, gc.IsNil)
}

func (s *metricsManagerSuite) TestNewMetricsManagerAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authorizer
	anAuthoriser.Tag = names.NewUnitTag("mysql/0")
	endPoint, err := metricsmanager.NewMetricsManagerAPI(s.State, nil, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
