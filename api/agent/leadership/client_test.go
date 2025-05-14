// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/leadership"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	coreleadership "github.com/juju/juju/core/leadership"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

/*
Test that the client is translating incoming parameters to the
service layer correctly, and also translates the results back
correctly.
*/

type ClientSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&ClientSuite{})

const (
	StubApplicationNm = "stub-application"
	StubUnitNm        = "stub-unit/0"
)

func (s *ClientSuite) apiCaller(c *tc.C, check func(request string, arg, result interface{}) error) base.APICaller {
	return apitesting.APICallerFunc(func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, tc.Equals, "LeadershipService")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		return check(request, arg, result)
	})
}

func (s *ClientSuite) TestClaimLeadershipTranslation(c *tc.C) {

	const claimTime = 5 * time.Hour
	numStubCalls := 0

	apiCaller := s.apiCaller(c, func(request string, arg, result interface{}) error {
		numStubCalls++
		c.Check(request, tc.Equals, "ClaimLeadership")
		c.Check(arg, tc.DeepEquals, params.ClaimLeadershipBulkParams{
			Params: []params.ClaimLeadershipParams{{
				ApplicationTag:  "application-stub-application",
				UnitTag:         "unit-stub-unit-0",
				DurationSeconds: claimTime.Seconds(),
			}},
		})
		switch result := result.(type) {
		case *params.ClaimLeadershipBulkResults:
			result.Results = []params.ErrorResult{{}}
		default:
			c.Fatalf("bad result type: %T", result)
		}
		return nil
	})

	client := leadership.NewClient(apiCaller)
	err := client.ClaimLeadership(c.Context(), StubApplicationNm, StubUnitNm, claimTime)
	c.Check(err, tc.ErrorIsNil)
	c.Check(numStubCalls, tc.Equals, 1)
}

func (s *ClientSuite) TestClaimLeadershipDeniedError(c *tc.C) {

	numStubCalls := 0
	apiCaller := s.apiCaller(c, func(_ string, _, result interface{}) error {
		numStubCalls++
		switch result := result.(type) {
		case *params.ClaimLeadershipBulkResults:
			result.Results = []params.ErrorResult{{Error: &params.Error{
				Message: "blah",
				Code:    params.CodeLeadershipClaimDenied,
			}}}
		default:
			c.Fatalf("bad result type: %T", result)
		}
		return nil
	})

	client := leadership.NewClient(apiCaller)
	err := client.ClaimLeadership(c.Context(), StubApplicationNm, StubUnitNm, 0)
	c.Check(numStubCalls, tc.Equals, 1)
	c.Check(err, tc.Equals, coreleadership.ErrClaimDenied)
}

func (s *ClientSuite) TestClaimLeadershipUnknownError(c *tc.C) {

	errMsg := "I'm trying!"
	numStubCalls := 0
	apiCaller := s.apiCaller(c, func(_ string, _, result interface{}) error {
		numStubCalls++
		switch result := result.(type) {
		case *params.ClaimLeadershipBulkResults:
			result.Results = []params.ErrorResult{{Error: &params.Error{
				Message: errMsg,
			}}}
		default:
			c.Fatalf("bad result type: %T", result)
		}
		return nil
	})

	client := leadership.NewClient(apiCaller)
	err := client.ClaimLeadership(c.Context(), StubApplicationNm, StubUnitNm, 0)
	c.Check(numStubCalls, tc.Equals, 1)
	c.Check(err, tc.ErrorMatches, errMsg)
}

func (s *ClientSuite) TestClaimLeadershipFacadeCallError(c *tc.C) {
	errMsg := "well, I just give up."
	numStubCalls := 0
	apiCaller := s.apiCaller(c, func(_ string, _, _ interface{}) error {
		numStubCalls++
		return errors.New(errMsg)
	})

	client := leadership.NewClient(apiCaller)
	err := client.ClaimLeadership(c.Context(), StubApplicationNm, StubUnitNm, 0)
	c.Check(numStubCalls, tc.Equals, 1)
	c.Check(err, tc.ErrorMatches, "error making a leadership claim: "+errMsg)
}

func (s *ClientSuite) TestBlockUntilLeadershipReleasedTranslation(c *tc.C) {

	numStubCalls := 0
	apiCaller := s.apiCaller(c, func(request string, arg, result interface{}) error {
		numStubCalls++
		c.Check(request, tc.Equals, "BlockUntilLeadershipReleased")
		c.Check(arg, tc.DeepEquals, names.NewApplicationTag(StubApplicationNm))
		switch result := result.(type) {
		case *params.ErrorResult:
		default:
			c.Fatalf("bad result type: %T", result)
		}
		return nil
	})

	client := leadership.NewClient(apiCaller)
	err := client.BlockUntilLeadershipReleased(c.Context(), StubApplicationNm)

	c.Check(numStubCalls, tc.Equals, 1)
	c.Check(err, tc.ErrorIsNil)
}

func (s *ClientSuite) TestBlockUntilLeadershipReleasedError(c *tc.C) {

	numStubCalls := 0
	apiCaller := s.apiCaller(c, func(_ string, _, result interface{}) error {
		numStubCalls++
		switch result := result.(type) {
		case *params.ErrorResult:
			*result = params.ErrorResult{Error: &params.Error{Message: "splat"}}
		default:
			c.Fatalf("bad result type: %T", result)
		}
		return nil
	})

	client := leadership.NewClient(apiCaller)
	err := client.BlockUntilLeadershipReleased(c.Context(), StubApplicationNm)

	c.Check(numStubCalls, tc.Equals, 1)
	c.Check(err, tc.ErrorMatches, "error blocking on leadership release: splat")
}

func (s *ClientSuite) TestBlockUntilLeadershipReleasedFacadeCallError(c *tc.C) {
	errMsg := "well, I just give up."
	numStubCalls := 0
	apiCaller := s.apiCaller(c, func(_ string, _, _ interface{}) error {
		numStubCalls++
		return errors.New(errMsg)
	})

	client := leadership.NewClient(apiCaller)
	err := client.BlockUntilLeadershipReleased(c.Context(), StubApplicationNm)
	c.Check(numStubCalls, tc.Equals, 1)
	c.Check(err, tc.ErrorMatches, "error blocking on leadership release: "+errMsg)
}
