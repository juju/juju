// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricstorage_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/metricstorage"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type metricsManagerSuite struct {
	jujutesting.JujuConnSuite

	client  *metricstorage.Client
	charm   *state.Charm
	service *state.Service
	unit    *state.Unit

	stateAPI *api.State
}

var _ = gc.Suite(&metricsManagerSuite{})

func (s *metricsManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.charm = s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	s.service = s.Factory.MakeService(c, &factory.ServiceParams{Charm: s.charm})
	s.unit = s.Factory.MakeUnit(c, &factory.UnitParams{Service: s.service, SetCharmURL: true})

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.stateAPI = s.OpenAPIAs(c, s.unit.Tag(), password)

	s.client = metricstorage.NewClient(s.stateAPI, s.unit.UnitTag())
	c.Assert(s.client, gc.NotNil)
}

func (s *metricsManagerSuite) TestSendMetricBatchPatch(c *gc.C) {
	metrics := []params.Metric{{"pings", "5", time.Now().UTC()}}
	uuid := utils.MustNewUUID().String()
	batch := params.MetricBatch{
		UUID:     uuid,
		CharmURL: s.charm.URL().String(),
		Created:  time.Now(),
		Metrics:  metrics,
	}

	var called bool
	metricstorage.PatchFacadeCall(s, s.client, func(request string, args, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "AddMetricBatches")
		c.Assert(args.(params.MetricBatchParams).Batches, gc.HasLen, 1)
		c.Assert(args.(params.MetricBatchParams).Batches[0].Batch, gc.DeepEquals, batch)
		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		return nil
	})

	err := s.client.AddMetricBatches([]params.MetricBatch{batch})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *metricsManagerSuite) TestSendMetricBatchFail(c *gc.C) {
	var called bool
	metricstorage.PatchFacadeCall(s, s.client, func(request string, args, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "AddMetricBatches")
		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		result.Results[0].Error = common.ServerError(common.ErrPerm)
		return nil
	})
	metrics := []params.Metric{{"pings", "5", time.Now().UTC()}}
	uuid := utils.MustNewUUID().String()
	batch := params.MetricBatch{
		UUID:     uuid,
		CharmURL: s.charm.URL().String(),
		Created:  time.Now(),
		Metrics:  metrics,
	}

	err := s.client.AddMetricBatches([]params.MetricBatch{batch})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(called, jc.IsTrue)
}

func (s *metricsManagerSuite) TestSendMetricBatch(c *gc.C) {
	uuid := utils.MustNewUUID().String()
	now := time.Now().Round(time.Second).UTC()
	metrics := []params.Metric{{"pings", "5", now}}
	batch := params.MetricBatch{
		UUID:     uuid,
		CharmURL: s.charm.URL().String(),
		Created:  now,
		Metrics:  metrics,
	}

	err := s.client.AddMetricBatches([]params.MetricBatch{batch})
	c.Assert(err, jc.ErrorIsNil)

	batches, err := s.State.MetricBatches()
	c.Assert(err, gc.IsNil)
	c.Assert(batches, gc.HasLen, 1)
	c.Assert(batches[0].UUID(), gc.Equals, uuid)
	c.Assert(batches[0].Sent(), jc.IsFalse)
	c.Assert(batches[0].CharmURL(), gc.Equals, s.charm.URL().String())
	c.Assert(batches[0].Metrics(), gc.HasLen, 1)
	c.Assert(batches[0].Metrics()[0].Key, gc.Equals, "pings")
	c.Assert(batches[0].Metrics()[0].Key, gc.Equals, "pings")
	c.Assert(batches[0].Metrics()[0].Value, gc.Equals, "5")
}

func (s *metricsManagerSuite) TestInvalidUnit(c *gc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: s.service, SetCharmURL: true})

	client := metricstorage.NewClient(s.stateAPI, unit.UnitTag())
	c.Assert(client, gc.NotNil)

	metrics := []params.Metric{{"pings", "5", time.Now().UTC()}}
	uuid := utils.MustNewUUID().String()
	batch := params.MetricBatch{
		UUID:     uuid,
		CharmURL: s.charm.URL().String(),
		Created:  time.Now(),
		Metrics:  metrics,
	}

	err := client.AddMetricBatches([]params.MetricBatch{batch})
	c.Assert(err, gc.ErrorMatches, "permission denied")

}
