// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/params"
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

func (s *provisionerSuite) TestUpdateUnits(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASOperatorProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "UpdateApplicationsUnits")
		c.Assert(a, jc.DeepEquals, params.UpdateApplicationUnitArgs{
			Args: []params.UpdateApplicationUnits{
				{
					ApplicationTag: "application-app",
					Units: []params.ApplicationUnitParams{
						{Id: "uuid", Address: "address", Ports: []string{"port"},
							Status: "active", Info: "message"},
					},
				},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	err := client.UpdateUnits(params.UpdateApplicationUnits{
		ApplicationTag: names.NewApplicationTag("app").String(),
		Units: []params.ApplicationUnitParams{
			{Id: "uuid", Address: "address", Ports: []string{"port"},
				Status: "active", Info: "message"},
		},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *provisionerSuite) TestUpdateUnitsCount(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	err := client.UpdateUnits(params.UpdateApplicationUnits{
		ApplicationTag: names.NewApplicationTag("app").String(),
		Units: []params.ApplicationUnitParams{
			{Id: "uuid", Address: "address"},
		},
	})
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
}
