// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/juju/controller"
	jujutesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2/txn"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/upgradesteps"
	"github.com/juju/juju/apiserver/facades/agent/upgradesteps/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

type upgradeStepsSuite struct {
	jujutesting.BaseSuite

	api        *upgradesteps.UpgradeStepsAPI
	authorizer *facademocks.MockAuthorizer
	entity     *mocks.MockEntity
	resources  *facademocks.MockResources
	state      *mocks.MockUpgradeStepsState
}

type machineUpgradeStepsSuite struct {
	upgradeStepsSuite

	tag     names.Tag
	arg     params.Entity
	machine *mocks.MockMachine
}

var _ = gc.Suite(&machineUpgradeStepsSuite{})

func (s *machineUpgradeStepsSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("0/kvm/0")
	s.arg = params.Entity{Tag: s.tag.String()}
	s.BaseSuite.SetUpTest(c)
}

func (s *machineUpgradeStepsSuite) TestResetKVMMachineModificationStatusIdle(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectContainerType(instance.KVM)
	s.expectModificationStatus(status.Error)
	s.expectSetModificationStatus(nil)

	s.setupFacadeAPI(c)

	result, err := s.api.ResetKVMMachineModificationStatusIdle(s.arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{})
}

func (s *machineUpgradeStepsSuite) TestResetKVMMachineModificationStatusIdleSetError(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectContainerType(instance.KVM)
	s.expectModificationStatus(status.Error)
	s.expectSetModificationStatus(errors.NotFoundf("testing"))

	s.setupFacadeAPI(c)

	result, err := s.api.ResetKVMMachineModificationStatusIdle(s.arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{
		Error: &params.Error{
			Message: "testing not found",
			Code:    "not found",
		},
	})
}

func (s *machineUpgradeStepsSuite) TestResetKVMMachineModificationStatusIdleKVMIdle(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectContainerType(instance.KVM)
	s.expectModificationStatus(status.Idle)

	s.setupFacadeAPI(c)

	_, err := s.api.ResetKVMMachineModificationStatusIdle(s.arg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *machineUpgradeStepsSuite) TestResetKVMMachineModificationStatusIdleLXD(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectContainerType(instance.LXD)

	s.setupFacadeAPI(c)

	_, err := s.api.ResetKVMMachineModificationStatusIdle(s.arg)
	c.Assert(err, jc.ErrorIsNil)
}

type unitUpgradeStepsSuite struct {
	upgradeStepsSuite
	tag1  names.Tag
	tag2  names.Tag
	unit1 *mocks.MockUnit
	unit2 *mocks.MockUnit
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

	results, err := s.api.WriteAgentState(args)
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

	results, err := s.api.WriteAgentState(args)
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
	ctlr := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctlr)
	s.entity = mocks.NewMockEntity(ctlr)
	s.state = mocks.NewMockUpgradeStepsState(ctlr)
	s.resources = facademocks.NewMockResources(ctlr)

	return ctlr
}

func (s *upgradeStepsSuite) expectAuthCalls() {
	aExp := s.authorizer.EXPECT()
	aExp.AuthMachineAgent().Return(true).AnyTimes()
	aExp.AuthController().Return(true).AnyTimes()
}

func (s *upgradeStepsSuite) setupFacadeAPI(c *gc.C) {
	api, err := upgradesteps.NewUpgradeStepsAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	s.api = api
}

func (s *machineUpgradeStepsSuite) setup(c *gc.C) *gomock.Controller {
	ctlr := s.upgradeStepsSuite.setup(c)
	s.machine = mocks.NewMockMachine(ctlr)

	s.expectAuthCalls()
	s.expectFindEntityMachine()

	return ctlr
}

func (s *machineUpgradeStepsSuite) expectAuthCalls() {
	s.upgradeStepsSuite.expectAuthCalls()
	aExp := s.authorizer.EXPECT()
	aExp.GetAuthTag().Return(s.tag).AnyTimes()
}

func (s *machineUpgradeStepsSuite) expectFindEntityMachine() {
	mEntity := machineEntityShim{
		Machine: s.machine,
		Entity:  s.entity,
	}
	s.state.EXPECT().FindEntity(s.tag.(names.MachineTag)).Return(mEntity, nil)
}

func (s *machineUpgradeStepsSuite) expectContainerType(cType instance.ContainerType) {
	mExp := s.machine.EXPECT()
	mExp.ContainerType().Return(cType)
}

func (s *machineUpgradeStepsSuite) expectModificationStatus(sValue status.Status) {
	mExp := s.machine.EXPECT()
	mExp.ModificationStatus().Return(status.StatusInfo{Status: sValue}, nil)
}

func (s *machineUpgradeStepsSuite) expectSetModificationStatus(err error) {
	mExp := s.machine.EXPECT()
	mExp.SetModificationStatus(status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Data:    nil,
	}).Return(err)
}

func (s *unitUpgradeStepsSuite) setup(c *gc.C) *gomock.Controller {
	ctlr := s.upgradeStepsSuite.setup(c)
	s.unit1 = mocks.NewMockUnit(ctlr)
	s.unit2 = mocks.NewMockUnit(ctlr)

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

	s.state.EXPECT().ControllerConfig().Return(ctrlCfg, nil)

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

type machineEntityShim struct {
	upgradesteps.Machine
	state.Entity
}

type unitEntityShim struct {
	upgradesteps.Unit
	state.Entity
}

type dummyOp struct {
}

func (d dummyOp) Build(attempt int) ([]txn.Op, error) { return nil, nil }
func (d dummyOp) Done(_ error) error                  { return nil }
