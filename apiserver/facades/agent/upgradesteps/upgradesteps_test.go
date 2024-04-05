// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/upgradesteps"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	jujutesting "github.com/juju/juju/testing"
)

type upgradeStepsSuite struct {
	jujutesting.BaseSuite

	api              *upgradesteps.UpgradeStepsAPI
	authorizer       *MockAuthorizer
	entity           *MockEntity
	resources        *MockResources
	state            *MockUpgradeStepsState
	ctrlConfigGetter *MockControllerConfigGetter
}

type unitUpgradeStepsSuite struct {
	upgradeStepsSuite
	tag1  names.Tag
	tag2  names.Tag
	unit1 *MockUnit
	unit2 *MockUnit
}

var _ = gc.Suite(&unitUpgradeStepsSuite{})

func (s *unitUpgradeStepsSuite) SetUpTest(c *gc.C) {
	s.tag1 = names.NewUnitTag("ubuntu/0")
	s.tag2 = names.NewUnitTag("ubuntu/1")
	s.BaseSuite.SetUpTest(c)
}

func (s *unitUpgradeStepsSuite) TestWriteAgentState(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectSetAndApplyStateOperation(nil, nil)
	s.setupFacadeAPI(c)

	str1 := "foo"
	str2 := "bar"
	args := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{
			{Tag: s.tag1.String(), UniterState: &str1, StorageState: &str2},
			{Tag: s.tag2.String(), UniterState: &str2},
		},
	}

	results, err := s.api.WriteAgentState(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}, {}}})
}

func (s *unitUpgradeStepsSuite) TestWriteAgentStateError(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectSetAndApplyStateOperation(nil, errors.NotFoundf("testing"))
	s.setupFacadeAPI(c)

	str1 := "foo"
	str2 := "bar"
	args := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{
			{Tag: s.tag1.String(), UniterState: &str1, StorageState: &str2},
			{Tag: s.tag2.String(), UniterState: &str2},
		},
	}

	results, err := s.api.WriteAgentState(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{
		{},
		{
			Error: &params.Error{
				Message: "testing not found",
				Code:    "not found",
			},
		}}})
}

func (s *upgradeStepsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = NewMockAuthorizer(ctrl)
	s.entity = NewMockEntity(ctrl)
	s.state = NewMockUpgradeStepsState(ctrl)
	s.resources = NewMockResources(ctrl)
	s.ctrlConfigGetter = NewMockControllerConfigGetter(ctrl)

	return ctrl
}

func (s *upgradeStepsSuite) expectAuthCalls() {
	aExp := s.authorizer.EXPECT()
	aExp.AuthMachineAgent().Return(true).AnyTimes()
	aExp.AuthController().Return(true).AnyTimes()
}

func (s *upgradeStepsSuite) setupFacadeAPI(c *gc.C) {
	api, err := upgradesteps.NewUpgradeStepsAPI(
		s.state,
		s.ctrlConfigGetter,
		s.resources,
		s.authorizer,
		jujutesting.NewCheckLogger(c),
	)
	c.Assert(err, gc.IsNil)
	s.api = api
}

func (s *unitUpgradeStepsSuite) setup(c *gc.C) *gomock.Controller {
	ctlr := s.upgradeStepsSuite.setup(c)
	s.unit1 = NewMockUnit(ctlr)
	s.unit2 = NewMockUnit(ctlr)

	s.expectAuthCalls()
	s.expectFindEntityUnits()

	return ctlr
}

func (s *unitUpgradeStepsSuite) expectAuthCalls() {
	s.upgradeStepsSuite.expectAuthCalls()
	aExp := s.authorizer.EXPECT()
	aExp.GetAuthTag().Return(s.tag1).AnyTimes()
	aExp.GetAuthTag().Return(s.tag2).AnyTimes()
}

func (s *unitUpgradeStepsSuite) expectFindEntityUnits() {
	u1Entity := unitEntityShim{
		Unit:   s.unit1,
		Entity: s.entity,
	}
	s.state.EXPECT().FindEntity(s.tag1.(names.UnitTag)).Return(u1Entity, nil)

	u2Entity := unitEntityShim{
		Unit:   s.unit2,
		Entity: s.entity,
	}
	s.state.EXPECT().FindEntity(s.tag2.(names.UnitTag)).Return(u2Entity, nil)
}

func (s *unitUpgradeStepsSuite) expectSetAndApplyStateOperation(err1, err2 error) {
	ctrlCfg := controller.Config{
		controller.MaxCharmStateSize: 123,
		controller.MaxAgentStateSize: 456,
	}

	s.ctrlConfigGetter.EXPECT().ControllerConfig(gomock.Any()).Return(ctrlCfg, nil)

	expLimits := state.UnitStateSizeLimits{
		MaxCharmStateSize: 123,
		MaxAgentStateSize: 456,
	}

	us := state.NewUnitState()
	us.SetUniterState("foo")
	us.SetStorageState("bar")

	op1 := dummyOp{}
	s.unit1.EXPECT().SetStateOperation(us, expLimits).Return(op1)
	s.state.EXPECT().ApplyOperation(op1).Return(err1)

	us = state.NewUnitState()
	us.SetUniterState("bar")
	op2 := dummyOp{}
	s.unit2.EXPECT().SetStateOperation(us, expLimits).Return(op2)
	s.state.EXPECT().ApplyOperation(op2).Return(err2)
}

type unitEntityShim struct {
	upgradesteps.Unit
	state.Entity
}

type dummyOp struct {
}

func (d dummyOp) Build(attempt int) ([]txn.Op, error) { return nil, nil }
func (d dummyOp) Done(_ error) error                  { return nil }

type kvmMachineUpgradeStepsSuite struct {
	upgradeStepsSuite

	tag1    names.Tag
	machine *MockMachine
}

var _ = gc.Suite(&kvmMachineUpgradeStepsSuite{})

func (s *kvmMachineUpgradeStepsSuite) SetUpTest(c *gc.C) {
	s.upgradeStepsSuite.SetUpTest(c)
	s.tag1 = names.NewMachineTag("0")
}

func (s *kvmMachineUpgradeStepsSuite) setup(c *gc.C) *gomock.Controller {
	ctlr := s.upgradeStepsSuite.setup(c)

	s.expectAuthCalls()
	s.machine = NewMockMachine(ctlr)
	return ctlr
}

func (s *kvmMachineUpgradeStepsSuite) TestResetKVMMachineModificationStatusIdleNoop(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthCalls()
	s.setupFacadeAPI(c)
	api := &upgradesteps.UpgradeStepsAPIV2{s.api}

	s.expectFindEntityMachine(instance.LXD)

	result, err := api.ResetKVMMachineModificationStatusIdle(context.Background(), params.Entity{Tag: s.tag1.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *kvmMachineUpgradeStepsSuite) TestResetKVMMachineModificationStatusIdleKVMUnsupported(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthCalls()
	s.setupFacadeAPI(c)
	api := &upgradesteps.UpgradeStepsAPIV2{s.api}

	s.expectFindEntityMachine("kvm")

	_, err := api.ResetKVMMachineModificationStatusIdle(context.Background(), params.Entity{Tag: s.tag1.String()})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *kvmMachineUpgradeStepsSuite) expectFindEntityMachine(t instance.ContainerType) {
	s.machine.EXPECT().ContainerType().Return(t)
	m := machineEntityShim{
		Machine: s.machine,
		Entity:  s.entity,
	}

	s.state.EXPECT().FindEntity(s.tag1.(names.MachineTag)).Return(m, nil)
}

type machineEntityShim struct {
	upgradesteps.Machine
	state.Entity
}
