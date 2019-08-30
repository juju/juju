// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jujutesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

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

	tag names.Tag
	arg params.Entity

	api        *upgradesteps.UpgradeStepsAPI
	authorizer *facademocks.MockAuthorizer
	entity     *mocks.MockEntity
	machine    *mocks.MockMachine
	resources  *facademocks.MockResources
	state      *mocks.MockUpgradeStepsState
}

var _ = gc.Suite(&upgradeStepsSuite{})

func (s *upgradeStepsSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("0/kvm/0")
	s.arg = params.Entity{Tag: s.tag.String()}
	s.BaseSuite.SetUpTest(c)
}

func (s *upgradeStepsSuite) TestResetKVMMachineModificationStatusIdle(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthCalls()
	s.expectFindEntity()
	s.expectContainerType(instance.KVM)
	s.expectModificationStatus(status.Error)
	s.expectSetModificationStatus(nil)

	s.setupFacadeAPI(c)

	result, err := s.api.ResetKVMMachineModificationStatusIdle(s.arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResult{})
}

func (s *upgradeStepsSuite) TestResetKVMMachineModificationStatusIdleSetError(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthCalls()
	s.expectFindEntity()
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

func (s *upgradeStepsSuite) TestResetKVMMachineModificationStatusIdleKVMIdle(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthCalls()
	s.expectFindEntity()
	s.expectContainerType(instance.KVM)
	s.expectModificationStatus(status.Idle)

	s.setupFacadeAPI(c)

	_, err := s.api.ResetKVMMachineModificationStatusIdle(s.arg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradeStepsSuite) TestResetKVMMachineModificationStatusIdleLXD(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthCalls()
	s.expectFindEntity()
	s.expectContainerType(instance.LXD)

	s.setupFacadeAPI(c)

	_, err := s.api.ResetKVMMachineModificationStatusIdle(s.arg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradeStepsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.entity = mocks.NewMockEntity(ctrl)
	s.machine = mocks.NewMockMachine(ctrl)
	s.state = mocks.NewMockUpgradeStepsState(ctrl)
	s.resources = facademocks.NewMockResources(ctrl)

	return ctrl
}

func (s *upgradeStepsSuite) setupFacadeAPI(c *gc.C) {
	api, err := upgradesteps.NewUpgradeStepsAPI(s.state, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	s.api = api
}

func (s *upgradeStepsSuite) expectFindEntity() {
	mEntity := machineEntityShim{
		Machine: s.machine,
		Entity:  s.entity,
	}
	s.state.EXPECT().FindEntity(s.tag.(names.MachineTag)).Return(mEntity, nil)
}

func (s *upgradeStepsSuite) expectAuthCalls() {
	aExp := s.authorizer.EXPECT()
	aExp.AuthMachineAgent().Return(true).AnyTimes()
	aExp.AuthController().Return(true).AnyTimes()
	aExp.GetAuthTag().Return(s.tag).AnyTimes()
}

func (s *upgradeStepsSuite) expectContainerType(cType instance.ContainerType) {
	mExp := s.machine.EXPECT()
	mExp.ContainerType().Return(cType)
}

func (s *upgradeStepsSuite) expectModificationStatus(sValue status.Status) {
	mExp := s.machine.EXPECT()
	mExp.ModificationStatus().Return(status.StatusInfo{Status: sValue}, nil)
}

func (s *upgradeStepsSuite) expectSetModificationStatus(err error) {
	mExp := s.machine.EXPECT()
	mExp.SetModificationStatus(status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Data:    nil,
	}).Return(err)
}

type machineEntityShim struct {
	upgradesteps.Machine
	state.Entity
}
