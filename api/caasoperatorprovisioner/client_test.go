// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
)

type provisionerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&provisionerSuite{})

func newClient(f basetesting.APICallerFunc) *caasoperatorprovisioner.Client {
	return caasoperatorprovisioner.NewClient(basetesting.BestVersionCaller{f, 5})
}

func (s *provisionerSuite) TestWatchApplications(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASOperatorProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "WatchApplications")
		c.Assert(a, gc.IsNil)
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})
	_, err := client.WatchApplications()
	c.Check(err, gc.ErrorMatches, "FAIL")
	c.Check(called, jc.IsTrue)
}

func (s *provisionerSuite) TestSetPasswords(c *gc.C) {
	passwords := []caasoperatorprovisioner.ApplicationPassword{
		{Name: "app", Password: "secret"},
	}
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASOperatorProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "SetPasswords")
		c.Assert(a, jc.DeepEquals, params.EntityPasswords{
			Changes: []params.EntityPassword{{Tag: "application-app", Password: "secret"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	result, err := client.SetPasswords(passwords)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result.Combine(), jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *provisionerSuite) TestSetPasswordsCount(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	passwords := []caasoperatorprovisioner.ApplicationPassword{
		{Name: "app", Password: "secret"},
	}
	_, err := client.SetPasswords(passwords)
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *provisionerSuite) TestLife(c *gc.C) {
	tag := names.NewApplicationTag("app")
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASOperatorProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "Life")
		c.Check(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: tag.String(),
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{
				Life: params.Alive,
			}},
		}
		return nil
	})

	client := caasoperatorprovisioner.NewClient(apiCaller)
	lifeValue, err := client.Life(tag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lifeValue, gc.Equals, life.Alive)
}

func (s *provisionerSuite) TestLifeError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{{Error: &params.Error{
				Code:    params.CodeNotFound,
				Message: "bletch",
			}}},
		}
		return nil
	})

	client := caasoperatorprovisioner.NewClient(apiCaller)
	_, err := client.Life("gitlab")
	c.Assert(err, gc.ErrorMatches, "bletch")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *provisionerSuite) TestLifeInvalidApplicationName(c *gc.C) {
	client := caasoperatorprovisioner.NewClient(basetesting.APICallerFunc(func(_ string, _ int, _, _ string, _, _ interface{}) error {
		return errors.New("should not be called")
	}))
	_, err := client.Life("")
	c.Assert(err, gc.ErrorMatches, `application name "" not valid`)
}

func (s *provisionerSuite) TestLifeCount(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		*(result.(*params.LifeResults)) = params.LifeResults{
			Results: []params.LifeResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	_, err := client.Life("gitlab")
	c.Check(err, gc.ErrorMatches, `expected 1 result, got 2`)
}
