// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
)

type upgradeSeriesSuite struct {
	testing.IsolationSuite
	tag names.Tag
}

var _ = gc.Suite(&upgradeSeriesSuite{})

func (s *upgradeSeriesSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("0")
}

func (s *upgradeSeriesSuite) TestWatchUpgradeSeriesNotifications(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "WatchUpgradeSeriesNotifications")
		c.Assert(args, jc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				NotifyWatcherId: "1",
				Error:           nil,
			}},
		}
		return nil
	}
	apiCaller := apitesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			c.Check(objType, gc.Equals, "NotifyWatcher")
			c.Check(id, gc.Equals, "1")
			c.Check(request, gc.Equals, "Next")
			c.Check(a, gc.IsNil)
			return nil
		},
	)
	facadeCaller.ReturnRawAPICaller = apitesting.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 1}

	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	_, err := api.WatchUpgradeSeriesNotifications()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesStatusPrepare(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "UpgradeSeriesPrepareStatus")
		c.Assert(args, jc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{{Status: "completed"}},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	watchResult, err := api.UpgradeSeriesStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watchResult, gc.DeepEquals, []string{"completed"})
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesStatusWithComplete(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "UpgradeSeriesPrepareStatus")
		c.Assert(args, jc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{{
				Status: "completed",
				Error:  nil,
			}},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	watchResult, err := api.UpgradeSeriesStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watchResult, gc.DeepEquals, []string{"completed"})
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesStatusNotFound(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "UpgradeSeriesPrepareStatus")
		c.Assert(args, jc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{{
				Error: &params.Error{
					Code:    params.CodeNotFound,
					Message: `testing`,
				},
			}},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	watchResult, err := api.UpgradeSeriesStatus()
	c.Assert(err, gc.ErrorMatches, "testing")
	c.Check(errors.IsNotFound(err), jc.IsTrue)
	c.Check(watchResult, gc.HasLen, 0)
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesStatusMultiple(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "UpgradeSeriesPrepareStatus")
		c.Assert(args, jc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{
				{Status: "Started"},
				{Status: "Completed"},
			},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	watchResult, err := api.UpgradeSeriesStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(watchResult, jc.SameContents, []string{"Started", "Completed"})
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatus(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "SetUpgradeSeriesPrepareStatus")
		c.Assert(args, jc.DeepEquals, params.SetUpgradeSeriesStatusParams{
			Params: []params.SetUpgradeSeriesStatusParam{{
				Entity: params.Entity{Tag: s.tag.String()},
				Status: "Errored",
			}},
		})
		*(response.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: nil,
			}},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	err := api.SetUpgradeSeriesStatus(string(model.UnitErrored))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatusNotOne(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "SetUpgradeSeriesPrepareStatus")
		c.Assert(args, jc.DeepEquals, params.SetUpgradeSeriesStatusParams{
			Params: []params.SetUpgradeSeriesStatusParam{{
				Entity: params.Entity{Tag: s.tag.String()},
				Status: "Errored",
			}},
		})
		*(response.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	err := api.SetUpgradeSeriesStatus(string(model.UnitErrored))
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatusResultError(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "SetUpgradeSeriesPrepareStatus")
		c.Assert(args, jc.DeepEquals, params.SetUpgradeSeriesStatusParams{
			Params: []params.SetUpgradeSeriesStatusParam{{
				Entity: params.Entity{Tag: s.tag.String()},
				Status: "Errored",
			}},
		})
		*(response.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{
				Error: &params.Error{Message: "error in call"},
			}},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	err := api.SetUpgradeSeriesStatus(string(model.UnitErrored))
	c.Assert(err, gc.ErrorMatches, "error in call")
}
