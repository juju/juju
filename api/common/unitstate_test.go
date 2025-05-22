// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type unitStateSuite struct {
	testhelpers.IsolationSuite
	tag names.UnitTag
}

func TestUnitStateSuite(t *stdtesting.T) {
	tc.Run(t, &unitStateSuite{})
}

func (s *unitStateSuite) SetUpTest(c *tc.C) {
	s.tag = names.NewUnitTag("test-unit/0")
}

func (s *unitStateSuite) TestSetStateSingleResult(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "SetState")
		c.Assert(args.(params.SetUnitStateArgs).Args, tc.HasLen, 1)
		c.Assert(args.(params.SetUnitStateArgs).Args[0].Tag, tc.Equals, s.tag.String())
		c.Assert(*args.(params.SetUnitStateArgs).Args[0].CharmState, tc.DeepEquals, map[string]string{"one": "two"})
		*(response.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: nil,
			}},
		}
		return nil
	}
	api := common.NewUniterStateAPI(&facadeCaller, s.tag)
	err := api.SetState(c.Context(), params.SetUnitStateArg{
		CharmState: &map[string]string{"one": "two"},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *unitStateSuite) TestSetStateReturnsQuotaExceededError(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		result := response.(*params.ErrorResults)
		result.Results = []params.ErrorResult{{
			Error: apiservererrors.ServerError(errors.NewQuotaLimitExceeded(nil, "cake slice limit exceeded; try again later")),
		}}
		return nil
	}

	// The client should reconstruct the quota error from the server response
	api := common.NewUniterStateAPI(&facadeCaller, s.tag)
	err := api.SetState(c.Context(), params.SetUnitStateArg{
		CharmState: &map[string]string{"one": "two"},
	})
	c.Assert(err, tc.ErrorIs, errors.QuotaLimitExceeded, tc.Commentf("expected the client to reconstruct QuotaLimitExceeded error from server response"))
}

func (s *unitStateSuite) TestSetStateMultipleReturnsError(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "SetState")
		c.Assert(args.(params.SetUnitStateArgs).Args, tc.HasLen, 1)
		c.Assert(args.(params.SetUnitStateArgs).Args[0].Tag, tc.Equals, s.tag.String())
		c.Assert(*args.(params.SetUnitStateArgs).Args[0].CharmState, tc.DeepEquals, map[string]string{"one": "two"})
		*(response.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: nil},
				{Error: nil},
			},
		}
		return nil
	}

	api := common.NewUniterStateAPI(&facadeCaller, s.tag)
	err := api.SetState(c.Context(), params.SetUnitStateArg{
		CharmState: &map[string]string{"one": "two"},
	})
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}

func (s *unitStateSuite) TestStateSingleResult(c *tc.C) {
	expectedCharmState := map[string]string{
		"one":   "two",
		"three": "four",
	}
	expectedUniterState := "testing"

	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "State")
		*(response.(*params.UnitStateResults)) = params.UnitStateResults{
			Results: []params.UnitStateResult{{
				UniterState: expectedUniterState,
				CharmState:  expectedCharmState,
			}}}
		return nil
	}

	api := common.NewUniterStateAPI(&facadeCaller, s.tag)
	obtainedUnitState, err := api.State(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(expectedCharmState, tc.DeepEquals, obtainedUnitState.CharmState)
	c.Assert(expectedUniterState, tc.DeepEquals, obtainedUnitState.UniterState)
}

func (s *unitStateSuite) TestStateMultipleReturnsError(c *tc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testhelpers.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, tc.Equals, "State")
		*(response.(*params.UnitStateResults)) = params.UnitStateResults{
			Results: []params.UnitStateResult{
				{Error: &params.Error{Code: params.CodeNotFound, Message: `testing`}},
				{Error: &params.Error{Code: params.CodeNotFound, Message: `other`}},
			}}
		return nil
	}

	api := common.NewUniterStateAPI(&facadeCaller, s.tag)
	_, err := api.State(c.Context())
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}
