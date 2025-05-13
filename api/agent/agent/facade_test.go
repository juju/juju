// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/agent"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type FacadeSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&FacadeSuite{})

func (s *FacadeSuite) TestLifeCallError(c *tc.C) {
	apiCaller := apiCaller(c, func(request string, arg, _ interface{}) error {
		c.Check(request, tc.Equals, "GetEntities")
		c.Check(arg, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-omg",
			}},
		})
		return errors.New("splat")
	})
	facade, err := agent.NewConnFacade(apiCaller)
	c.Assert(err, tc.ErrorIsNil)

	life, err := facade.Life(context.Background(), names.NewApplicationTag("omg"))
	c.Check(err, tc.ErrorMatches, "splat")
	c.Check(life, tc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeNoResult(c *tc.C) {
	result := params.AgentGetEntitiesResults{}
	facade, err := agent.NewConnFacade(lifeChecker(c, result))
	c.Assert(err, tc.ErrorIsNil)

	life, err := facade.Life(context.Background(), names.NewApplicationTag("omg"))
	c.Check(err, tc.ErrorMatches, "expected 1 result, got 0")
	c.Check(life, tc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeOversizedResult(c *tc.C) {
	result := params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{{}, {}},
	}
	facade, err := agent.NewConnFacade(lifeChecker(c, result))
	c.Assert(err, tc.ErrorIsNil)

	life, err := facade.Life(context.Background(), names.NewApplicationTag("omg"))
	c.Check(err, tc.ErrorMatches, "expected 1 result, got 2")
	c.Check(life, tc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeRandomError(c *tc.C) {
	result := params.AgentGetEntitiesResult{
		Error: &params.Error{Message: "squish"},
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, tc.ErrorMatches, "squish")
	c.Check(life, tc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeErrNotFound(c *tc.C) {
	result := params.AgentGetEntitiesResult{
		Error: &params.Error{Code: params.CodeNotFound},
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, tc.Equals, agent.ErrDenied)
	c.Check(life, tc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeErrUnauthorized(c *tc.C) {
	result := params.AgentGetEntitiesResult{
		Error: &params.Error{Code: params.CodeUnauthorized},
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, tc.Equals, agent.ErrDenied)
	c.Check(life, tc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeUnknown(c *tc.C) {
	result := params.AgentGetEntitiesResult{
		Life: "revenant",
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, tc.ErrorMatches, `unknown life value "revenant"`)
	c.Check(life, tc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeAlive(c *tc.C) {
	result := params.AgentGetEntitiesResult{
		Life: "alive",
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, tc.ErrorIsNil)
	c.Check(life, tc.Equals, agent.Alive)
}

func (s *FacadeSuite) TestLifeDying(c *tc.C) {
	result := params.AgentGetEntitiesResult{
		Life: "dying",
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, tc.ErrorIsNil)
	c.Check(life, tc.Equals, agent.Dying)
}

func (s *FacadeSuite) TestLifeDead(c *tc.C) {
	result := params.AgentGetEntitiesResult{
		Life: "dead",
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, tc.ErrorIsNil)
	c.Check(life, tc.Equals, agent.Dead)
}

func (s *FacadeSuite) TestSetPasswordCallError(c *tc.C) {
	apiCaller := apiCaller(c, func(request string, arg, _ interface{}) error {
		c.Check(request, tc.Equals, "SetPasswords")
		c.Check(arg, tc.DeepEquals, params.EntityPasswords{
			Changes: []params.EntityPassword{{
				Tag:      "application-omg",
				Password: "seekr1t",
			}},
		})
		return errors.New("splat")
	})
	facade, err := agent.NewConnFacade(apiCaller)
	c.Assert(err, tc.ErrorIsNil)

	err = facade.SetPassword(context.Background(), names.NewApplicationTag("omg"), "seekr1t")
	c.Check(err, tc.ErrorMatches, "splat")
}

func (s *FacadeSuite) TestSetPasswordNoResult(c *tc.C) {
	result := params.ErrorResults{}
	facade, err := agent.NewConnFacade(passwordChecker(c, result))
	c.Assert(err, tc.ErrorIsNil)

	err = facade.SetPassword(context.Background(), names.NewApplicationTag("omg"), "blah")
	c.Check(err, tc.ErrorMatches, "expected 1 result, got 0")
}

func (s *FacadeSuite) TestSetPasswordOversizedResult(c *tc.C) {
	result := params.ErrorResults{
		Results: []params.ErrorResult{{}, {}},
	}
	facade, err := agent.NewConnFacade(passwordChecker(c, result))
	c.Assert(err, tc.ErrorIsNil)

	err = facade.SetPassword(context.Background(), names.NewApplicationTag("omg"), "blah")
	c.Check(err, tc.ErrorMatches, "expected 1 result, got 2")
}

func (s *FacadeSuite) TestSetPasswordRandomError(c *tc.C) {
	result := params.ErrorResult{
		Error: &params.Error{Message: "squish"},
	}
	err := testPasswordAPIResult(c, result)
	c.Check(err, tc.ErrorMatches, "squish")
}

func (s *FacadeSuite) TestSetPasswordErrDead(c *tc.C) {
	result := params.ErrorResult{
		Error: &params.Error{Code: params.CodeDead},
	}
	err := testPasswordAPIResult(c, result)
	c.Check(err, tc.Equals, agent.ErrDenied)
}

func (s *FacadeSuite) TestSetPasswordErrNotFound(c *tc.C) {
	result := params.ErrorResult{
		Error: &params.Error{Code: params.CodeNotFound},
	}
	err := testPasswordAPIResult(c, result)
	c.Check(err, tc.Equals, agent.ErrDenied)
}

func (s *FacadeSuite) TestSetPasswordErrUnauthorized(c *tc.C) {
	result := params.ErrorResult{
		Error: &params.Error{Code: params.CodeUnauthorized},
	}
	err := testPasswordAPIResult(c, result)
	c.Check(err, tc.Equals, agent.ErrDenied)
}

func (s *FacadeSuite) TestSetPasswordSuccess(c *tc.C) {
	result := params.ErrorResult{}
	err := testPasswordAPIResult(c, result)
	c.Check(err, tc.ErrorIsNil)
}

func testLifeAPIResult(c *tc.C, result params.AgentGetEntitiesResult) (agent.Life, error) {
	facade, err := agent.NewConnFacade(lifeChecker(c, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{result},
	}))
	c.Assert(err, tc.ErrorIsNil)

	return facade.Life(context.Background(), names.NewApplicationTag("omg"))
}

func lifeChecker(c *tc.C, result params.AgentGetEntitiesResults) base.APICaller {
	return apiCaller(c, func(_ string, _, out interface{}) error {
		typed, ok := out.(*params.AgentGetEntitiesResults)
		c.Assert(ok, tc.IsTrue)
		*typed = result
		return nil
	})
}

func testPasswordAPIResult(c *tc.C, result params.ErrorResult) error {
	facade, err := agent.NewConnFacade(passwordChecker(c, params.ErrorResults{
		Results: []params.ErrorResult{result},
	}))
	c.Assert(err, tc.ErrorIsNil)

	return facade.SetPassword(context.Background(), names.NewApplicationTag("omg"), "blah")
}

func passwordChecker(c *tc.C, result params.ErrorResults) base.APICaller {
	return apiCaller(c, func(_ string, _, out interface{}) error {
		typed, ok := out.(*params.ErrorResults)
		c.Assert(ok, tc.IsTrue)
		*typed = result
		return nil
	})
}

func apiCaller(c *tc.C, check func(request string, arg, result interface{}) error) base.APICaller {
	return apitesting.APICallerFunc(func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, tc.Equals, "Agent")
		c.Check(version, tc.Equals, 0) // because of BestFacadeVersion test infrastructure
		c.Check(id, tc.Equals, "")
		return check(request, arg, result)
	})
}
