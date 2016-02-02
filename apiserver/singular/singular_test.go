// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/singular"
	"github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
)

type SingularSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SingularSuite{})

func (s *SingularSuite) TestRequiresEnvironManager(c *gc.C) {
	auth := mockAuth{nonManager: true}
	facade, err := singular.NewFacade(nil, auth)
	c.Check(facade, gc.IsNil)
	c.Check(err, gc.Equals, common.ErrPerm)
}

func (s *SingularSuite) TestAcceptsEnvironManager(c *gc.C) {
	backend := &mockBackend{}
	facade, err := singular.NewFacade(backend, mockAuth{})
	c.Check(facade, gc.NotNil)
	c.Check(err, jc.ErrorIsNil)

	backend.stub.CheckCallNames(c)
}

func (s *SingularSuite) TestInvalidClaims(c *gc.C) {
	breakers := []func(claim *params.SingularClaim){
		func(claim *params.SingularClaim) { claim.ModelTag = "" },
		func(claim *params.SingularClaim) { claim.ModelTag = "machine-123" },
		func(claim *params.SingularClaim) { claim.ModelTag = "environ-blargle" },
		func(claim *params.SingularClaim) { claim.ControllerTag = "" },
		func(claim *params.SingularClaim) { claim.ControllerTag = "machine-456" },
		func(claim *params.SingularClaim) { claim.ControllerTag = coretesting.ModelTag.String() },
		func(claim *params.SingularClaim) { claim.Duration = time.Second - time.Millisecond },
		func(claim *params.SingularClaim) { claim.Duration = time.Minute + time.Millisecond },
	}
	count := len(breakers)

	var claims params.SingularClaims
	claims.Claims = make([]params.SingularClaim, count)
	for i, breaker := range breakers {
		claim := params.SingularClaim{
			ModelTag:      coretesting.ModelTag.String(),
			ControllerTag: "machine-123",
			Duration:      time.Minute,
		}
		breaker(&claim)
		claims.Claims[i] = claim
	}

	backend := &mockBackend{}
	facade, err := singular.NewFacade(backend, mockAuth{})
	c.Assert(err, jc.ErrorIsNil)
	result := facade.Claim(claims)
	c.Assert(result.Results, gc.HasLen, count)

	for i, result := range result.Results {
		c.Logf("checking claim %d", i)
		checkDenied(c, result)
	}
	backend.stub.CheckCallNames(c)
}

func (s *SingularSuite) TestValidClaims(c *gc.C) {
	durations := []time.Duration{
		time.Second,
		10 * time.Second,
		30 * time.Second,
		time.Minute,
	}
	errors := []error{
		nil,
		errors.New("pow!"),
		lease.ErrClaimDenied,
		nil,
	}
	count := len(durations)
	if len(errors) != count {
		c.Fatalf("please fix your test data")
	}

	var claims params.SingularClaims
	claims.Claims = make([]params.SingularClaim, count)
	expectCalls := []testing.StubCall{}
	for i, duration := range durations {
		claims.Claims[i] = params.SingularClaim{
			ModelTag:      coretesting.ModelTag.String(),
			ControllerTag: "machine-123",
			Duration:      duration,
		}
		expectCalls = append(expectCalls, testing.StubCall{
			FuncName: "Claim",
			Args: []interface{}{
				coretesting.ModelTag.Id(),
				"machine-123",
				durations[i],
			},
		})
	}

	backend := &mockBackend{}
	backend.stub.SetErrors(errors...)
	facade, err := singular.NewFacade(backend, mockAuth{})
	c.Assert(err, jc.ErrorIsNil)
	result := facade.Claim(claims)
	c.Assert(result.Results, gc.HasLen, count)

	for i, err := range result.Results {
		switch errors[i] {
		case nil:
			c.Check(err.Error, gc.IsNil)
		case lease.ErrClaimDenied:
			c.Check(err.Error, jc.Satisfies, params.IsCodeLeaseClaimDenied)
		default:
			c.Check(err.Error.Error(), gc.Equals, errors[i].Error())
		}
	}
	backend.stub.CheckCalls(c, expectCalls)
}

func (s *SingularSuite) TestWait(c *gc.C) {
	waits := params.Entities{
		Entities: []params.Entity{{
			"machine-123", // rejected
		}, {
			"grarble floop", // rejected
		}, {
			coretesting.ModelTag.String(), // stub-error
		}, {
			coretesting.ModelTag.String(), // success
		}},
	}
	count := len(waits.Entities)

	backend := &mockBackend{}
	backend.stub.SetErrors(errors.New("zap!"), nil)
	facade, err := singular.NewFacade(backend, mockAuth{})
	c.Assert(err, jc.ErrorIsNil)
	result := facade.Wait(waits)
	c.Assert(result.Results, gc.HasLen, count)

	checkDenied(c, result.Results[0])
	checkDenied(c, result.Results[1])
	c.Check(result.Results[2].Error, gc.ErrorMatches, "zap!")
	c.Check(result.Results[3].Error, gc.IsNil)

	backend.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "WaitUntilExpired",
		Args:     []interface{}{coretesting.ModelTag.Id()},
	}, {
		FuncName: "WaitUntilExpired",
		Args:     []interface{}{coretesting.ModelTag.Id()},
	}})
}

func checkDenied(c *gc.C, result params.ErrorResult) {
	c.Check(result.Error, gc.ErrorMatches, "permission denied")
	c.Check(result.Error, jc.Satisfies, params.IsCodeUnauthorized)
}
