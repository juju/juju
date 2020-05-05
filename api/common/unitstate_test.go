// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/common"
	srvcommon "github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

type unitStateSuite struct {
	testing.IsolationSuite
	tag names.UnitTag
}

var _ = gc.Suite(&unitStateSuite{})

func (s *unitStateSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewUnitTag("test-unit/0")
}

func (s *unitStateSuite) TestSetStateSingleResult(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "SetState")
		c.Assert(args.(params.SetUnitStateArgs).Args, gc.HasLen, 1)
		c.Assert(args.(params.SetUnitStateArgs).Args[0].Tag, gc.Equals, s.tag.String())
		c.Assert(*args.(params.SetUnitStateArgs).Args[0].CharmState, jc.DeepEquals, map[string]string{"one": "two"})
		*(response.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: nil,
			}},
		}
		return nil
	}
	api := common.NewUniterStateAPI(&facadeCaller, s.tag)
	err := api.SetState(params.SetUnitStateArg{
		CharmState: &map[string]string{"one": "two"},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitStateSuite) TestSetStateReturnsQuotaExceededError(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		result := response.(*params.ErrorResults)
		result.Results = []params.ErrorResult{{
			Error: srvcommon.ServerError(errors.NewQuotaLimitExceeded(nil, "cake slice limit exceeded; try again later")),
		}}
		return nil
	}

	// The client should reconstruct the quota error from the server response
	api := common.NewUniterStateAPI(&facadeCaller, s.tag)
	err := api.SetState(params.SetUnitStateArg{
		CharmState: &map[string]string{"one": "two"},
	})
	c.Assert(err, jc.Satisfies, errors.IsQuotaLimitExceeded, gc.Commentf("expected the client to reconstruct QuotaLimitExceeded error from server response"))
}

func (s *unitStateSuite) TestSetStateMultipleReturnsError(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "SetState")
		c.Assert(args.(params.SetUnitStateArgs).Args, gc.HasLen, 1)
		c.Assert(args.(params.SetUnitStateArgs).Args[0].Tag, gc.Equals, s.tag.String())
		c.Assert(*args.(params.SetUnitStateArgs).Args[0].CharmState, jc.DeepEquals, map[string]string{"one": "two"})
		*(response.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: nil},
				{Error: nil},
			},
		}
		return nil
	}

	api := common.NewUniterStateAPI(&facadeCaller, s.tag)
	err := api.SetState(params.SetUnitStateArg{
		CharmState: &map[string]string{"one": "two"},
	})
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *unitStateSuite) TestStateSingleResult(c *gc.C) {
	expectedCharmState := map[string]string{
		"one":   "two",
		"three": "four",
	}
	expectedUniterState := "testing"

	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "State")
		*(response.(*params.UnitStateResults)) = params.UnitStateResults{
			Results: []params.UnitStateResult{{
				UniterState: expectedUniterState,
				CharmState:  expectedCharmState,
			}}}
		return nil
	}

	api := common.NewUniterStateAPI(&facadeCaller, s.tag)
	obtainedUnitState, err := api.State()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(expectedCharmState, gc.DeepEquals, obtainedUnitState.CharmState)
	c.Assert(expectedUniterState, gc.DeepEquals, obtainedUnitState.UniterState)
}

func (s *unitStateSuite) TestStateMultipleReturnsError(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "State")
		*(response.(*params.UnitStateResults)) = params.UnitStateResults{
			Results: []params.UnitStateResult{
				{Error: &params.Error{Code: params.CodeNotFound, Message: `testing`}},
				{Error: &params.Error{Code: params.CodeNotFound, Message: `other`}},
			}}
		return nil
	}

	api := common.NewUniterStateAPI(&facadeCaller, s.tag)
	_, err := api.State()
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}
