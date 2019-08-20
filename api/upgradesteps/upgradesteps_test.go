// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/upgradesteps"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/testing"
)

type upgradeStepsSuite struct {
	jujutesting.BaseSuite

	tag names.Tag
	arg params.Entity

	fCaller *mocks.MockFacadeCaller
}

var _ = gc.Suite(&upgradeStepsSuite{})

func (s *upgradeStepsSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("0/kvm/0")
	s.arg = params.Entity{Tag: s.tag.String()}
	s.BaseSuite.SetUpTest(c)
}

func (s *upgradeStepsSuite) TestResetKVMMachineModificationStatusIdle(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectResetKVMMachineModificationStatusIdleSuccess()

	client := upgradesteps.NewClientFromFacade(s.fCaller)
	err := client.ResetKVMMachineModificationStatusIdle(s.tag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradeStepsSuite) TestResetKVMMachineModificationStatusIdleError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectResetKVMMachineModificationStatusIdleError()

	client := upgradesteps.NewClientFromFacade(s.fCaller)
	err := client.ResetKVMMachineModificationStatusIdle(s.tag)
	c.Assert(err, gc.ErrorMatches, "did not find")
}

func (s *upgradeStepsSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.fCaller = mocks.NewMockFacadeCaller(ctrl)
	return ctrl
}

func (s *upgradeStepsSuite) expectResetKVMMachineModificationStatusIdleSuccess() {
	fExp := s.fCaller.EXPECT()
	resultSource := params.ErrorResult{}
	fExp.FacadeCall("ResetKVMMachineModificationStatusIdle", s.arg, gomock.Any()).SetArg(2, resultSource)
}

func (s *upgradeStepsSuite) expectResetKVMMachineModificationStatusIdleError() {
	fExp := s.fCaller.EXPECT()
	resultSource := params.ErrorResult{
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "did not find",
		},
	}
	fExp.FacadeCall("ResetKVMMachineModificationStatusIdle", s.arg, gomock.Any()).SetArg(2, resultSource)
}
