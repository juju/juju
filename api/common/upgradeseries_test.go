// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
		c.Assert(name, gc.Equals, "UpgradeSeriesUnitStatus")
		c.Assert(args, jc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{{Status: model.UpgradeSeriesCompleted}},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	watchResult, err := api.UpgradeSeriesUnitStatus()
	c.Assert(err, jc.ErrorIsNil)

	exp := []model.UpgradeSeriesStatus{model.UpgradeSeriesCompleted}
	c.Check(watchResult, jc.SameContents, exp)
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesStatusWithComplete(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "UpgradeSeriesUnitStatus")
		c.Assert(args, jc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{{Status: model.UpgradeSeriesCompleted}},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	watchResult, err := api.UpgradeSeriesUnitStatus()
	c.Assert(err, jc.ErrorIsNil)

	exp := []model.UpgradeSeriesStatus{model.UpgradeSeriesCompleted}
	c.Check(watchResult, jc.SameContents, exp)
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesStatusNotFound(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "UpgradeSeriesUnitStatus")
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
	watchResult, err := api.UpgradeSeriesUnitStatus()
	c.Assert(err, gc.ErrorMatches, "testing")
	c.Check(errors.IsNotFound(err), jc.IsTrue)
	c.Check(watchResult, gc.HasLen, 0)
}

func (s *upgradeSeriesSuite) TestUpgradeSeriesStatusMultiple(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "UpgradeSeriesUnitStatus")
		c.Assert(args, jc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{
				{Status: "prepare started"},
				{Status: "prepare completed"},
			},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	watchResult, err := api.UpgradeSeriesUnitStatus()
	c.Assert(err, jc.ErrorIsNil)

	exp := []model.UpgradeSeriesStatus{model.UpgradeSeriesPrepareStarted, model.UpgradeSeriesPrepareCompleted}
	c.Check(watchResult, jc.SameContents, exp)
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatus(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "SetUpgradeSeriesUnitStatus")
		c.Assert(args, jc.DeepEquals, params.UpgradeSeriesStatusParams{
			Params: []params.UpgradeSeriesStatusParam{{
				Entity: params.Entity{Tag: s.tag.String()},
				Status: model.UpgradeSeriesError,
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
	err := api.SetUpgradeSeriesUnitStatus(model.UpgradeSeriesError, "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatusNotOne(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "SetUpgradeSeriesUnitStatus")
		c.Assert(args, jc.DeepEquals, params.UpgradeSeriesStatusParams{
			Params: []params.UpgradeSeriesStatusParam{{
				Entity: params.Entity{Tag: s.tag.String()},
				Status: model.UpgradeSeriesError,
			}},
		})
		*(response.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{},
		}
		return nil
	}
	api := common.NewUpgradeSeriesAPI(&facadeCaller, s.tag)
	err := api.SetUpgradeSeriesUnitStatus(model.UpgradeSeriesError, "")
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
}

func (s *upgradeSeriesSuite) TestSetUpgradeSeriesStatusResultError(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "SetUpgradeSeriesUnitStatus")
		c.Assert(args, jc.DeepEquals, params.UpgradeSeriesStatusParams{
			Params: []params.UpgradeSeriesStatusParam{{
				Entity: params.Entity{Tag: s.tag.String()},
				Status: model.UpgradeSeriesError,
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
	err := api.SetUpgradeSeriesUnitStatus(model.UpgradeSeriesError, "")
	c.Assert(err, gc.ErrorMatches, "error in call")
}
