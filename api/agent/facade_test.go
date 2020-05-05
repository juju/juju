// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/apiserver/params"
)

type FacadeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FacadeSuite{})

func (s *FacadeSuite) TestLifeCallError(c *gc.C) {
	apiCaller := apiCaller(c, func(request string, arg, _ interface{}) error {
		c.Check(request, gc.Equals, "GetEntities")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-omg",
			}},
		})
		return errors.New("splat")
	})
	facade, err := agent.NewConnFacade(apiCaller)
	c.Assert(err, jc.ErrorIsNil)

	life, err := facade.Life(names.NewApplicationTag("omg"))
	c.Check(err, gc.ErrorMatches, "splat")
	c.Check(life, gc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeNoResult(c *gc.C) {
	result := params.AgentGetEntitiesResults{}
	facade, err := agent.NewConnFacade(lifeChecker(c, result))
	c.Assert(err, jc.ErrorIsNil)

	life, err := facade.Life(names.NewApplicationTag("omg"))
	c.Check(err, gc.ErrorMatches, "expected 1 result, got 0")
	c.Check(life, gc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeOversizedResult(c *gc.C) {
	result := params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{{}, {}},
	}
	facade, err := agent.NewConnFacade(lifeChecker(c, result))
	c.Assert(err, jc.ErrorIsNil)

	life, err := facade.Life(names.NewApplicationTag("omg"))
	c.Check(err, gc.ErrorMatches, "expected 1 result, got 2")
	c.Check(life, gc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeRandomError(c *gc.C) {
	result := params.AgentGetEntitiesResult{
		Error: &params.Error{Message: "squish"},
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, gc.ErrorMatches, "squish")
	c.Check(life, gc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeErrNotFound(c *gc.C) {
	result := params.AgentGetEntitiesResult{
		Error: &params.Error{Code: params.CodeNotFound},
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, gc.Equals, agent.ErrDenied)
	c.Check(life, gc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeErrUnauthorized(c *gc.C) {
	result := params.AgentGetEntitiesResult{
		Error: &params.Error{Code: params.CodeUnauthorized},
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, gc.Equals, agent.ErrDenied)
	c.Check(life, gc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeUnknown(c *gc.C) {
	result := params.AgentGetEntitiesResult{
		Life: "revenant",
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, gc.ErrorMatches, `unknown life value "revenant"`)
	c.Check(life, gc.Equals, agent.Life(""))
}

func (s *FacadeSuite) TestLifeAlive(c *gc.C) {
	result := params.AgentGetEntitiesResult{
		Life: "alive",
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, jc.ErrorIsNil)
	c.Check(life, gc.Equals, agent.Alive)
}

func (s *FacadeSuite) TestLifeDying(c *gc.C) {
	result := params.AgentGetEntitiesResult{
		Life: "dying",
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, jc.ErrorIsNil)
	c.Check(life, gc.Equals, agent.Dying)
}

func (s *FacadeSuite) TestLifeDead(c *gc.C) {
	result := params.AgentGetEntitiesResult{
		Life: "dead",
	}
	life, err := testLifeAPIResult(c, result)
	c.Check(err, jc.ErrorIsNil)
	c.Check(life, gc.Equals, agent.Dead)
}

func (s *FacadeSuite) TestSetPasswordCallError(c *gc.C) {
	apiCaller := apiCaller(c, func(request string, arg, _ interface{}) error {
		c.Check(request, gc.Equals, "SetPasswords")
		c.Check(arg, jc.DeepEquals, params.EntityPasswords{
			Changes: []params.EntityPassword{{
				Tag:      "application-omg",
				Password: "seekr1t",
			}},
		})
		return errors.New("splat")
	})
	facade, err := agent.NewConnFacade(apiCaller)
	c.Assert(err, jc.ErrorIsNil)

	err = facade.SetPassword(names.NewApplicationTag("omg"), "seekr1t")
	c.Check(err, gc.ErrorMatches, "splat")
}

func (s *FacadeSuite) TestSetPasswordNoResult(c *gc.C) {
	result := params.ErrorResults{}
	facade, err := agent.NewConnFacade(passwordChecker(c, result))
	c.Assert(err, jc.ErrorIsNil)

	err = facade.SetPassword(names.NewApplicationTag("omg"), "blah")
	c.Check(err, gc.ErrorMatches, "expected 1 result, got 0")
}

func (s *FacadeSuite) TestSetPasswordOversizedResult(c *gc.C) {
	result := params.ErrorResults{
		Results: []params.ErrorResult{{}, {}},
	}
	facade, err := agent.NewConnFacade(passwordChecker(c, result))
	c.Assert(err, jc.ErrorIsNil)

	err = facade.SetPassword(names.NewApplicationTag("omg"), "blah")
	c.Check(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *FacadeSuite) TestSetPasswordRandomError(c *gc.C) {
	result := params.ErrorResult{
		Error: &params.Error{Message: "squish"},
	}
	err := testPasswordAPIResult(c, result)
	c.Check(err, gc.ErrorMatches, "squish")
}

func (s *FacadeSuite) TestSetPasswordErrDead(c *gc.C) {
	result := params.ErrorResult{
		Error: &params.Error{Code: params.CodeDead},
	}
	err := testPasswordAPIResult(c, result)
	c.Check(err, gc.Equals, agent.ErrDenied)
}

func (s *FacadeSuite) TestSetPasswordErrNotFound(c *gc.C) {
	result := params.ErrorResult{
		Error: &params.Error{Code: params.CodeNotFound},
	}
	err := testPasswordAPIResult(c, result)
	c.Check(err, gc.Equals, agent.ErrDenied)
}

func (s *FacadeSuite) TestSetPasswordErrUnauthorized(c *gc.C) {
	result := params.ErrorResult{
		Error: &params.Error{Code: params.CodeUnauthorized},
	}
	err := testPasswordAPIResult(c, result)
	c.Check(err, gc.Equals, agent.ErrDenied)
}

func (s *FacadeSuite) TestSetPasswordSuccess(c *gc.C) {
	result := params.ErrorResult{}
	err := testPasswordAPIResult(c, result)
	c.Check(err, jc.ErrorIsNil)
}

func testLifeAPIResult(c *gc.C, result params.AgentGetEntitiesResult) (agent.Life, error) {
	facade, err := agent.NewConnFacade(lifeChecker(c, params.AgentGetEntitiesResults{
		Entities: []params.AgentGetEntitiesResult{result},
	}))
	c.Assert(err, jc.ErrorIsNil)

	return facade.Life(names.NewApplicationTag("omg"))
}

func lifeChecker(c *gc.C, result params.AgentGetEntitiesResults) base.APICaller {
	return apiCaller(c, func(_ string, _, out interface{}) error {
		typed, ok := out.(*params.AgentGetEntitiesResults)
		c.Assert(ok, jc.IsTrue)
		*typed = result
		return nil
	})
}

func testPasswordAPIResult(c *gc.C, result params.ErrorResult) error {
	facade, err := agent.NewConnFacade(passwordChecker(c, params.ErrorResults{
		Results: []params.ErrorResult{result},
	}))
	c.Assert(err, jc.ErrorIsNil)

	return facade.SetPassword(names.NewApplicationTag("omg"), "blah")
}

func passwordChecker(c *gc.C, result params.ErrorResults) base.APICaller {
	return apiCaller(c, func(_ string, _, out interface{}) error {
		typed, ok := out.(*params.ErrorResults)
		c.Assert(ok, jc.IsTrue)
		*typed = result
		return nil
	})
}

func apiCaller(c *gc.C, check func(request string, arg, result interface{}) error) base.APICaller {
	return apitesting.APICallerFunc(func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, gc.Equals, "Agent")
		c.Check(version, gc.Equals, 0) // because of BestFacadeVersion test infrastructure
		c.Check(id, gc.Equals, "")
		return check(request, arg, result)
	})
}
