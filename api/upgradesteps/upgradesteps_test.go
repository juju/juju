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

	writeArgs []params.SetUnitStateArg

	fCaller *mocks.MockFacadeCaller
}

var _ = gc.Suite(&upgradeStepsSuite{})

func (s *upgradeStepsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *upgradeStepsSuite) TestResetKVMMachineModificationStatusIdle(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mTag := names.NewMachineTag("0/kvm/0")
	resetArg := params.Entity{Tag: mTag.String()}

	s.expectResetKVMMachineModificationStatusIdleSuccess(resetArg)

	client := upgradesteps.NewClientFromFacade(s.fCaller)
	err := client.ResetKVMMachineModificationStatusIdle(mTag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradeStepsSuite) TestResetKVMMachineModificationStatusIdleError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	mTag := names.NewMachineTag("0/kvm/0")
	resetArg := params.Entity{Tag: mTag.String()}

	s.expectResetKVMMachineModificationStatusIdleError(resetArg)

	client := upgradesteps.NewClientFromFacade(s.fCaller)
	err := client.ResetKVMMachineModificationStatusIdle(mTag)
	c.Assert(err, gc.ErrorMatches, "did not find")
}

func (s *upgradeStepsSuite) TestWriteAgentState(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uTag0 := names.NewUnitTag("test/0")
	uTag1 := names.NewUnitTag("test/1")
	str0 := "foo"
	str1 := "bar"
	args := params.SetUnitStateArgs{[]params.SetUnitStateArg{
		{Tag: uTag0.String(), UniterState: &str0},
		{Tag: uTag1.String(), UniterState: &str1},
	}}
	s.expectWriteAgentStateSuccess(c, args)

	client := upgradesteps.NewClientFromFacade(s.fCaller)
	err := client.WriteAgentState([]params.SetUnitStateArg{
		{Tag: uTag0.String(), UniterState: &str0},
		{Tag: uTag1.String(), UniterState: &str1},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradeStepsSuite) TestWriteAgentStateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uTag0 := names.NewUnitTag("test/0")
	str0 := "foo"
	args := params.SetUnitStateArgs{[]params.SetUnitStateArg{
		{Tag: uTag0.String(), UniterState: &str0},
	}}
	s.expectWriteAgentStateError(c, args)

	client := upgradesteps.NewClientFromFacade(s.fCaller)
	err := client.WriteAgentState([]params.SetUnitStateArg{
		{Tag: uTag0.String(), UniterState: &str0},
	})
	c.Assert(err, gc.ErrorMatches, "did not find")
}

func (s *upgradeStepsSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.fCaller = mocks.NewMockFacadeCaller(ctrl)
	return ctrl
}

func (s *upgradeStepsSuite) expectResetKVMMachineModificationStatusIdleSuccess(resetArg params.Entity) {
	fExp := s.fCaller.EXPECT()
	resultSource := params.ErrorResult{}
	fExp.FacadeCall("ResetKVMMachineModificationStatusIdle", resetArg, gomock.Any()).SetArg(2, resultSource)
}

func (s *upgradeStepsSuite) expectResetKVMMachineModificationStatusIdleError(resetArg params.Entity) {
	fExp := s.fCaller.EXPECT()
	resultSource := params.ErrorResult{
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "did not find",
		},
	}
	fExp.FacadeCall("ResetKVMMachineModificationStatusIdle", resetArg, gomock.Any()).SetArg(2, resultSource)
}

func (s *upgradeStepsSuite) expectWriteAgentStateSuccess(c *gc.C, args params.SetUnitStateArgs) {
	fExp := s.fCaller.EXPECT()
	resultSource := params.ErrorResults{}
	fExp.FacadeCall("WriteAgentState", unitStateMatcher{c, args}, gomock.Any()).SetArg(2, resultSource)
}

func (s *upgradeStepsSuite) expectWriteAgentStateError(c *gc.C, args params.SetUnitStateArgs) {
	fExp := s.fCaller.EXPECT()
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: "did not find",
		},
	}}}
	fExp.FacadeCall("WriteAgentState", unitStateMatcher{c, args}, gomock.Any()).SetArg(2, resultSource)
}

type unitStateMatcher struct {
	c        *gc.C
	expected params.SetUnitStateArgs
}

func (m unitStateMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(params.SetUnitStateArgs)
	if !ok {
		return false
	}

	m.c.Assert(obtained.Args, gc.HasLen, len(m.expected.Args))

	for _, obt := range obtained.Args {
		var found bool
		for _, exp := range m.expected.Args {
			if obt.Tag == exp.Tag {
				m.c.Assert(obt, jc.DeepEquals, exp)
				found = true
			}
		}
		m.c.Assert(found, jc.IsTrue, gc.Commentf("obtained tag %s, not found in expected data", obt.Tag))
	}

	return true
}

func (m unitStateMatcher) String() string {
	return "Match the contents of the UniterState pointer in params.SetUnitStateArg"
}
