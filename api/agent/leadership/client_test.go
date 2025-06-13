// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/leadership"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	coreleadership "github.com/juju/juju/core/leadership"
	"github.com/juju/juju/rpc/params"
)

/*
Test that the client is translating incoming parameters to the
service layer correctly, and also translates the results back
correctly.
*/

type ClientSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ClientSuite{})

const (
	StubApplicationNm = "stub-application"
	StubUnitNm        = "stub-unit/0"
)

func (s *ClientSuite) apiCaller(c *gc.C, check func(request string, arg, result interface{}) error) base.APICaller {
	return apitesting.APICallerFunc(func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, gc.Equals, "LeadershipService")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		return check(request, arg, result)
	})
}

func (s *ClientSuite) TestClaimLeadershipTranslation(c *gc.C) {

	const claimTime = 5 * time.Hour
	numStubCalls := 0

	apiCaller := s.apiCaller(c, func(request string, arg, result interface{}) error {
		numStubCalls++
		c.Check(request, gc.Equals, "ClaimLeadership")
		c.Check(arg, jc.DeepEquals, params.ClaimLeadershipBulkParams{
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
	err := client.ClaimLeadership(StubApplicationNm, StubUnitNm, claimTime)
	c.Check(err, jc.ErrorIsNil)
	c.Check(numStubCalls, gc.Equals, 1)
}

func (s *ClientSuite) TestClaimLeadershipDeniedError(c *gc.C) {

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
	err := client.ClaimLeadership(StubApplicationNm, StubUnitNm, 0)
	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(err, gc.Equals, coreleadership.ErrClaimDenied)
}

func (s *ClientSuite) TestClaimLeadershipUnknownError(c *gc.C) {

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
	err := client.ClaimLeadership(StubApplicationNm, StubUnitNm, 0)
	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(err, gc.ErrorMatches, errMsg)
}

func (s *ClientSuite) TestClaimLeadershipFacadeCallError(c *gc.C) {
	errMsg := "well, I just give up."
	numStubCalls := 0
	apiCaller := s.apiCaller(c, func(_ string, _, _ interface{}) error {
		numStubCalls++
		return errors.New(errMsg)
	})

	client := leadership.NewClient(apiCaller)
	err := client.ClaimLeadership(StubApplicationNm, StubUnitNm, 0)
	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(err, gc.ErrorMatches, "error making a leadership claim: "+errMsg)
}

func (s *ClientSuite) TestBlockUntilLeadershipReleasedTranslation(c *gc.C) {

	numStubCalls := 0
	apiCaller := s.apiCaller(c, func(request string, arg, result interface{}) error {
		numStubCalls++
		c.Check(request, gc.Equals, "BlockUntilLeadershipReleased")
		c.Check(arg, jc.DeepEquals, names.NewApplicationTag(StubApplicationNm))
		switch result := result.(type) {
		case *params.ErrorResult:
		default:
			c.Fatalf("bad result type: %T", result)
		}
		return nil
	})

	client := leadership.NewClient(apiCaller)
	err := client.BlockUntilLeadershipReleased(StubApplicationNm, nil)

	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(err, jc.ErrorIsNil)
}

func (s *ClientSuite) TestBlockUntilLeadershipReleasedError(c *gc.C) {

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
	err := client.BlockUntilLeadershipReleased(StubApplicationNm, nil)

	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(err, gc.ErrorMatches, "error blocking on leadership release: splat")
}

func (s *ClientSuite) TestBlockUntilLeadershipReleasedFacadeCallError(c *gc.C) {
	errMsg := "well, I just give up."
	numStubCalls := 0
	apiCaller := s.apiCaller(c, func(_ string, _, _ interface{}) error {
		numStubCalls++
		return errors.New(errMsg)
	})

	client := leadership.NewClient(apiCaller)
	err := client.BlockUntilLeadershipReleased(StubApplicationNm, nil)
	c.Check(numStubCalls, gc.Equals, 1)
	c.Check(err, gc.ErrorMatches, "error blocking on leadership release: "+errMsg)
}
