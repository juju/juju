// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"context"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/undertaker"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type UndertakerSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&UndertakerSuite{})

func (s *UndertakerSuite) TestModelInfo(c *tc.C) {
	var called bool
	client := s.mockClient(c, "ModelInfo", func(response interface{}) {
		called = true
		result := response.(*params.UndertakerModelInfoResult)
		result.Result = params.UndertakerModelInfo{}
	})

	result, err := client.ModelInfo(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
	c.Assert(result, tc.Equals, params.UndertakerModelInfoResult{})
}

func (s *UndertakerSuite) TestProcessDyingModel(c *tc.C) {
	var called bool
	client := s.mockClient(c, "ProcessDyingModel", func(response interface{}) {
		called = true
		c.Assert(response, tc.IsNil)
	})

	c.Assert(client.ProcessDyingModel(context.Background()), tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

func (s *UndertakerSuite) TestRemoveModel(c *tc.C) {
	var called bool
	client := s.mockClient(c, "RemoveModel", func(response interface{}) {
		called = true
		c.Assert(response, tc.IsNil)
	})

	err := client.RemoveModel(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

func (s *UndertakerSuite) mockClient(c *tc.C, expectedRequest string, callback func(response interface{})) *undertaker.Client {
	apiCaller := basetesting.APICallerFunc(func(
		objType string,
		version int,
		id, request string,
		args, response interface{},
	) error {
		c.Check(objType, tc.Equals, "Undertaker")
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, expectedRequest)

		a, ok := args.(params.Entities)
		c.Check(ok, tc.IsTrue)
		c.Check(a.Entities, tc.DeepEquals, []params.Entity{{Tag: coretesting.ModelTag.String()}})

		callback(response)
		return nil
	})
	client, err := undertaker.NewClient(apiCaller, nil)
	c.Assert(err, tc.ErrorIsNil)
	return client
}

func (s *UndertakerSuite) TestWatchModelResourcesCreatesWatcher(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(
		objType string,
		version int,
		id, request string,
		args, response interface{},
	) error {
		c.Check(objType, tc.Equals, "Undertaker")
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchModelResources")

		a, ok := args.(params.Entities)
		c.Check(ok, tc.IsTrue)
		c.Check(a.Entities, tc.DeepEquals, []params.Entity{{Tag: coretesting.ModelTag.String()}})

		resp, ok := response.(*params.NotifyWatchResults)
		c.Assert(ok, tc.IsTrue)
		resp.Results = []params.NotifyWatchResult{{
			NotifyWatcherId: "1001",
		}}
		return nil
	})

	expectWatcher := &fakeWatcher{}
	newWatcher := func(apiCaller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(apiCaller, tc.NotNil) // uncomparable
		c.Check(result, tc.Equals, params.NotifyWatchResult{
			NotifyWatcherId: "1001",
		})
		return expectWatcher
	}

	client, err := undertaker.NewClient(apiCaller, newWatcher)
	c.Assert(err, tc.ErrorIsNil)
	w, err := client.WatchModelResources(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.Equals, expectWatcher)
}

func (s *UndertakerSuite) TestWatchModelResourcesError(c *tc.C) {
	var called bool
	client := s.mockClient(c, "WatchModelResources", func(response interface{}) {
		called = true
		_, ok := response.(*params.NotifyWatchResults)
		c.Check(ok, tc.IsTrue)
	})

	w, err := client.WatchModelResources(context.Background())
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 0")
	c.Assert(w, tc.IsNil)
	c.Assert(called, tc.IsTrue)
}

type fakeWatcher struct {
	watcher.NotifyWatcher
}
