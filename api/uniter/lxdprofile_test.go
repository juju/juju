// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
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
		c.Assert(args, jc.DeepEquals, params.LXDProfileUpgrade{
			Entities: []params.Entity{
				{Tag: s.tag.String()},
			},
			ApplicationName: "foo-bar",
		})
		*(response.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				StringsWatcherId: "1",
				Error:            nil,
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
			c.Check(objType, gc.Equals, "StringsWatcher")
			c.Check(id, gc.Equals, "1")
			c.Check(request, gc.Equals, "Next")
			c.Check(a, gc.IsNil)
			return nil
		},
	)
	facadeCaller.ReturnRawAPICaller = apitesting.BestVersionCaller{APICallerFunc: apiCaller, BestVersion: 1}

	api := uniter.NewLXDProfileAPI(&facadeCaller, s.tag)
	_, err := api.WatchLXDProfileUpgradeNotifications("foo-bar")
	c.Assert(err, jc.ErrorIsNil)
}
