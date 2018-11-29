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
	"github.com/juju/juju/core/lxdprofile"
)

type lxdProfileSuite struct {
	testing.IsolationSuite
	tag names.Tag
}

var _ = gc.Suite(&lxdProfileSuite{})

func (s *lxdProfileSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("0")
}

func (s *lxdProfileSuite) TestWatchLXDProfileUpgradeNotifications(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "WatchLXDProfileUpgradeNotifications")
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

	api := common.NewLXDProfileAPI(&facadeCaller, s.tag)
	_, err := api.WatchLXDProfileUpgradeNotifications()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *lxdProfileSuite) TestLXDProfileStatusWithSuccess(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "LXDProfileUnitStatus")
		c.Assert(args, jc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.LXDProfileStatusResults)) = params.LXDProfileStatusResults{
			Results: []params.LXDProfileStatusResult{{Status: lxdprofile.SuccessStatus}},
		}
		return nil
	}
	api := common.NewLXDProfileAPI(&facadeCaller, s.tag)
	watchResult, err := api.LXDProfileUnitStatus()
	c.Assert(err, jc.ErrorIsNil)

	exp := []string{lxdprofile.SuccessStatus}
	c.Check(watchResult, jc.SameContents, exp)
}

func (s *lxdProfileSuite) TestLXDProfileStatusNotFound(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "LXDProfileUnitStatus")
		c.Assert(args, jc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.LXDProfileStatusResults)) = params.LXDProfileStatusResults{
			Results: []params.LXDProfileStatusResult{{
				Error: &params.Error{
					Code:    params.CodeNotFound,
					Message: `testing`,
				},
			}},
		}
		return nil
	}
	api := common.NewLXDProfileAPI(&facadeCaller, s.tag)
	watchResult, err := api.LXDProfileUnitStatus()
	c.Assert(err, gc.ErrorMatches, "testing")
	c.Check(errors.IsNotFound(err), jc.IsTrue)
	c.Check(watchResult, gc.HasLen, 0)
}

func (s *lxdProfileSuite) TestLXDProfileStatusMultiple(c *gc.C) {
	facadeCaller := apitesting.StubFacadeCaller{Stub: &testing.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		c.Assert(name, gc.Equals, "LXDProfileUnitStatus")
		c.Assert(args, jc.DeepEquals, params.Entities{Entities: []params.Entity{
			{Tag: s.tag.String()},
		}})
		*(response.(*params.LXDProfileStatusResults)) = params.LXDProfileStatusResults{
			Results: []params.LXDProfileStatusResult{
				{Status: lxdprofile.SuccessStatus},
				{Status: lxdprofile.NotRequiredStatus},
			},
		}
		return nil
	}
	api := common.NewLXDProfileAPI(&facadeCaller, s.tag)
	watchResult, err := api.LXDProfileUnitStatus()
	c.Assert(err, jc.ErrorIsNil)

	exp := []string{lxdprofile.SuccessStatus, lxdprofile.NotRequiredStatus}
	c.Check(watchResult, jc.SameContents, exp)
}
