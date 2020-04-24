// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/singular"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
)

type APISuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&APISuite{})

var machine123 = names.NewMachineTag("123")

func (s *APISuite) TestBadClaimantTag(c *gc.C) {
	apiCaller := apiCaller(c, nil, nil)
	badTag := names.NewMachineTag("")
	api, err := singular.NewAPI(apiCaller, badTag, nil)
	c.Check(api, gc.IsNil)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, "claimant tag not valid")
}

func (s *APISuite) TestBadEntityTag(c *gc.C) {
	apiCaller := apiCaller(c, nil, nil)

	api, err := singular.NewAPI(apiCaller, machine123, nil)
	c.Check(api, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "nil entity supplied")

	api, err = singular.NewAPI(apiCaller, machine123, machine123)
	c.Check(api, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `invalid entity kind "machine" for singular API`)
}

func (s *APISuite) TestNoCalls(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, nil, nil)
	_, err := singular.NewAPI(apiCaller, machine123, coretesting.ControllerTag)
	c.Check(err, jc.ErrorIsNil)
	stub.CheckCallNames(c)
}

func (s *APISuite) TestClaimSuccess(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(result *params.ErrorResults) error {
		result.Results = []params.ErrorResult{{}}
		return nil
	})
	api, err := singular.NewAPI(apiCaller, machine123, coretesting.ModelTag)
	c.Assert(err, jc.ErrorIsNil)

	err = api.Claim(time.Minute)
	c.Check(err, jc.ErrorIsNil)
	checkCall(c, stub, "Claim", params.SingularClaims{
		Claims: []params.SingularClaim{{
			EntityTag:   "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
			ClaimantTag: "machine-123",
			Duration:    time.Minute,
		}},
	})
}

func (s *APISuite) TestClaimDenied(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(result *params.ErrorResults) error {
		result.Results = []params.ErrorResult{{
			Error: common.ServerError(lease.ErrClaimDenied),
		}}
		return nil
	})
	api, err := singular.NewAPI(apiCaller, machine123, coretesting.ModelTag)
	c.Assert(err, jc.ErrorIsNil)

	err = api.Claim(time.Hour)
	c.Check(err, gc.Equals, lease.ErrClaimDenied)
	checkCall(c, stub, "Claim", params.SingularClaims{
		Claims: []params.SingularClaim{{
			EntityTag:   "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
			ClaimantTag: "machine-123",
			Duration:    time.Hour,
		}},
	})
}

func (s *APISuite) TestClaimError(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(result *params.ErrorResults) error {
		result.Results = []params.ErrorResult{{
			Error: common.ServerError(errors.New("zap pow splat oof")),
		}}
		return nil
	})
	api, err := singular.NewAPI(apiCaller, machine123, coretesting.ModelTag)
	c.Assert(err, jc.ErrorIsNil)

	err = api.Claim(time.Second)
	c.Check(err, gc.ErrorMatches, "zap pow splat oof")
	checkCall(c, stub, "Claim", params.SingularClaims{
		Claims: []params.SingularClaim{{
			EntityTag:   "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
			ClaimantTag: "machine-123",
			Duration:    time.Second,
		}},
	})
}

func (s *APISuite) TestWaitSuccess(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(result *params.ErrorResults) error {
		result.Results = []params.ErrorResult{{}}
		return nil
	})
	api, err := singular.NewAPI(apiCaller, machine123, coretesting.ModelTag)
	c.Assert(err, jc.ErrorIsNil)

	err = api.Wait()
	c.Check(err, jc.ErrorIsNil)
	checkCall(c, stub, "Wait", params.Entities{
		Entities: []params.Entity{{
			Tag: "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		}},
	})
}

func (s *APISuite) TestWaitError(c *gc.C) {
	stub := &testing.Stub{}
	apiCaller := apiCaller(c, stub, func(result *params.ErrorResults) error {
		result.Results = []params.ErrorResult{{
			Error: common.ServerError(errors.New("crunch squelch")),
		}}
		return nil
	})
	api, err := singular.NewAPI(apiCaller, machine123, coretesting.ModelTag)
	c.Assert(err, jc.ErrorIsNil)

	err = api.Wait()
	c.Check(err, gc.ErrorMatches, "crunch squelch")
	checkCall(c, stub, "Wait", params.Entities{
		Entities: []params.Entity{{
			Tag: "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		}},
	})
}

type setResultFunc func(result *params.ErrorResults) error

func apiCaller(c *gc.C, stub *testing.Stub, setResult setResultFunc) base.APICaller {
	return basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			args, response interface{},
		) error {
			stub.AddCall(objType, version, id, request, args)
			result, ok := response.(*params.ErrorResults)
			c.Assert(ok, jc.IsTrue)
			return setResult(result)
		},
	)
}

func checkCall(c *gc.C, stub *testing.Stub, method string, args interface{}) {
	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Singular",
		Args:     []interface{}{0, "", method, args},
	}})
}
