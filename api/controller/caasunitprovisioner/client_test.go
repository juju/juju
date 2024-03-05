// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/caasunitprovisioner"
	"github.com/juju/juju/rpc/params"
)

type unitprovisionerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&unitprovisionerSuite{})

func newClient(f basetesting.APICallerFunc) *caasunitprovisioner.Client {
	return caasunitprovisioner.NewClient(basetesting.BestVersionCaller{f, 1})
}

func (s *unitprovisionerSuite) TestWatchApplications(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchApplications")
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResult{})
		*(result.(*params.StringsWatchResult)) = params.StringsWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	watcher, err := client.WatchApplications()
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *unitprovisionerSuite) TestWatchApplicationScale(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchApplicationsScale")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	watcher, err := client.WatchApplicationScale("gitlab")
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *unitprovisionerSuite) TestApplicationScale(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ApplicationsScale")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.IntResults{})
		*(result.(*params.IntResults)) = params.IntResults{
			Results: []params.IntResult{{
				Result: 5,
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	scale, err := client.ApplicationScale("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scale, gc.Equals, 5)
}

func (s *unitprovisionerSuite) TestUpdateApplicationService(c *gc.C) {
	var called bool
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		called = true
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
		c.Check(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "UpdateApplicationsService")
		c.Assert(a, jc.DeepEquals, params.UpdateApplicationServiceArgs{
			Args: []params.UpdateApplicationServiceArg{
				{
					ApplicationTag: "application-app",
					ProviderId:     "id",
					Addresses:      []params.Address{{Value: "10.0.0.1"}},
				},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		return nil
	})
	err := client.UpdateApplicationService(params.UpdateApplicationServiceArg{
		ApplicationTag: names.NewApplicationTag("app").String(),
		ProviderId:     "id",
		Addresses:      []params.Address{{Value: "10.0.0.1"}},
	})
	c.Check(err, jc.ErrorIsNil)
	c.Check(called, jc.IsTrue)
}

func (s *unitprovisionerSuite) TestUpdateApplicationServiceCount(c *gc.C) {
	client := newClient(func(objType string, version int, id, request string, a, result interface{}) error {
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: &params.Error{Message: "FAIL"}},
				{Error: &params.Error{Message: "FAIL"}},
			},
		}
		return nil
	})
	err := client.UpdateApplicationService(params.UpdateApplicationServiceArg{
		ApplicationTag: names.NewApplicationTag("app").String(),
		ProviderId:     "id",
		Addresses:      []params.Address{{Value: "10.0.0.1"}},
	})
	c.Check(err, gc.ErrorMatches, `expected 1 result\(s\), got 2`)
}

func (s *unitprovisionerSuite) TestWatchApplicationTrustHash(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchApplicationsTrustHash")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "FAIL"},
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	watcher, err := client.WatchApplicationTrustHash("gitlab")
	c.Assert(watcher, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "FAIL")
}

func (s *unitprovisionerSuite) TestApplicationTrust(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "CAASUnitProvisioner")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "ApplicationsTrust")
		c.Assert(arg, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				Tag: "application-gitlab",
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.BoolResults{})
		*(result.(*params.BoolResults)) = params.BoolResults{
			Results: []params.BoolResult{{
				Result: true,
			}},
		}
		return nil
	})

	client := caasunitprovisioner.NewClient(apiCaller)
	trust, err := client.ApplicationTrust("gitlab")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(trust, jc.IsTrue)
}
