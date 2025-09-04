// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsadder_test

import (
	"time"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/metricsadder"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	jujuFactory "github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&metricsAdderSuite{})

type metricsAdderSuite struct {
	jujutesting.JujuConnSuite

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	machine0       *state.Machine
	machine1       *state.Machine
	mysql          *state.Application
	mysqlUnit      *state.Unit
	meteredService *state.Application
	meteredCharm   *state.Charm
	meteredUnit    *state.Unit

	adder metricsadder.MetricsAdder
}

func (s *metricsAdderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.machine0 = s.Factory.MakeMachine(c, &jujuFactory.MachineParams{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits, state.JobManageModel},
	})
	s.machine1 = s.Factory.MakeMachine(c, &jujuFactory.MachineParams{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	mysqlCharm := s.Factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "mysql",
	})
	s.mysql = s.Factory.MakeApplication(c, &jujuFactory.ApplicationParams{
		Name:  "mysql",
		Charm: mysqlCharm,
	})
	s.mysqlUnit = s.Factory.MakeUnit(c, &jujuFactory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine0,
	})

	s.meteredCharm = s.Factory.MakeCharm(c, &jujuFactory.CharmParams{
		Name: "metered",
		URL:  "ch:amd64/quantal/metered",
	})
	s.meteredService = s.Factory.MakeApplication(c, &jujuFactory.ApplicationParams{
		Charm: s.meteredCharm,
	})
	s.meteredUnit = s.Factory.MakeUnit(c, &jujuFactory.UnitParams{
		Application: s.meteredService,
		SetCharmURL: true,
		Machine:     s.machine1,
	})

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming unit 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("1"),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	adder, err := metricsadder.NewMetricsAdderAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      s.authorizer,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.adder = adder
}

func (s *metricsAdderSuite) TestAddMetricsBatchNoOp(c *gc.C) {
	metrics := []params.Metric{{
		Key: "pings", Value: "5", Time: time.Now().UTC(),
	}, {
		Key: "pongs", Value: "6", Time: time.Now().UTC(), Labels: map[string]string{"foo": "bar"},
	}}
	uuid := utils.MustNewUUID().String()

	result, err := s.adder.AddMetricBatches(params.MetricBatchParams{
		Batches: []params.MetricBatchParam{{
			Tag: s.meteredUnit.Tag().String(),
			Batch: params.MetricBatch{
				UUID:     uuid,
				CharmURL: s.meteredCharm.URL(),
				Created:  time.Now(),
				Metrics:  metrics,
			}}}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
}
