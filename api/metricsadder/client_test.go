// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsadder_test

import (
	"time"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/metricsadder"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type metricsAdderSuite struct {
	jujutesting.JujuConnSuite

	adder *metricsadder.Client
}

var _ = gc.Suite(&metricsAdderSuite{})

func (s *metricsAdderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.adder = metricsadder.NewClient(s.APIState)
	c.Assert(s.adder, gc.NotNil)
}

func (s *metricsAdderSuite) TestAddMetricBatches(c *gc.C) {
	var called bool
	var callParams params.MetricBatchParams
	metricsadder.PatchFacadeCall(s, s.adder, func(request string, args, response interface{}) error {
		p, ok := args.(params.MetricBatchParams)
		c.Assert(ok, jc.IsTrue)
		callParams = p
		called = true
		c.Assert(request, gc.Equals, "AddMetricBatches")
		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		return nil
	})

	batches := []params.MetricBatchParam{{
		Tag: names.NewUnitTag("test-unit/0").String(),
		Batch: params.MetricBatch{
			UUID:     utils.MustNewUUID().String(),
			CharmURL: "test-charm-url",
			Created:  time.Now(),
			Metrics:  []params.Metric{{Key: "pings", Value: "5", Time: time.Now().UTC()}},
		},
	}}

	_, err := s.adder.AddMetricBatches(batches)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(callParams.Batches, gc.DeepEquals, batches)
}

func (s *metricsAdderSuite) TestAddMetricBatchesFails(c *gc.C) {
	var called bool
	metricsadder.PatchFacadeCall(s, s.adder, func(request string, args, response interface{}) error {
		_, ok := args.(params.MetricBatchParams)
		c.Assert(ok, jc.IsTrue)
		called = true
		c.Assert(request, gc.Equals, "AddMetricBatches")
		result := response.(*params.ErrorResults)
		result.Results = make([]params.ErrorResult, 1)
		result.Results[0].Error = common.ServerError(common.ErrPerm)
		return nil
	})

	batches := []params.MetricBatchParam{{
		Tag: names.NewUnitTag("test-unit/0").String(),
		Batch: params.MetricBatch{
			UUID:     utils.MustNewUUID().String(),
			CharmURL: "test-charm-url",
			Created:  time.Now(),
			Metrics:  []params.Metric{{Key: "pings", Value: "5", Time: time.Now().UTC()}},
		},
	}}

	results, err := s.adder.AddMetricBatches(batches)
	c.Assert(err, jc.ErrorIsNil)
	result, ok := results[batches[0].Batch.UUID]
	c.Assert(ok, jc.IsTrue)
	c.Assert(result.Error(), gc.Equals, "permission denied")
	c.Assert(called, jc.IsTrue)
}

type metricsAdderIntegrationSuite struct {
	jujutesting.JujuConnSuite

	adder   *metricsadder.Client
	unitTag names.Tag
}

var _ = gc.Suite(&metricsAdderIntegrationSuite{})

func (s *metricsAdderIntegrationSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	machine0 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})

	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "metered",
		URL:  "cs:quantal/metered",
	})
	meteredApp := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: meteredCharm,
	})
	meteredUnit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: meteredApp,
		SetCharmURL: true,
		Machine:     machine0,
	})

	state, _ := s.OpenAPIAsNewMachine(c)
	s.adder = metricsadder.NewClient(state)
	s.unitTag = meteredUnit.Tag()
}

func (s *metricsAdderIntegrationSuite) TestAddMetricBatches(c *gc.C) {
	batches := []params.MetricBatchParam{{
		Tag: s.unitTag.String(),
		Batch: params.MetricBatch{
			UUID:     utils.MustNewUUID().String(),
			CharmURL: "cs:quantal/metered",
			Created:  time.Now(),
			Metrics:  []params.Metric{{Key: "pings", Value: "5", Time: time.Now().UTC()}},
		},
	}}

	results, err := s.adder.AddMetricBatches(batches)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	result, ok := results[batches[0].Batch.UUID]
	c.Assert(ok, jc.IsTrue)
	c.Assert(result, gc.IsNil)

	stateBatches, err := s.State.AllMetricBatches()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateBatches, gc.HasLen, 1)
	c.Assert(stateBatches[0].CharmURL(), gc.Equals, batches[0].Batch.CharmURL)
	c.Assert(stateBatches[0].UUID(), gc.Equals, batches[0].Batch.UUID)
	c.Assert(stateBatches[0].ModelUUID(), gc.Equals, s.State.ModelUUID())
	c.Assert(stateBatches[0].Unit(), gc.Equals, s.unitTag.Id())
}
