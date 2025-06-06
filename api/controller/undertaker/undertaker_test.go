// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/undertaker"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type UndertakerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UndertakerSuite{})

func (s *UndertakerSuite) TestModelInfo(c *gc.C) {
	var called bool
	client := s.mockClient(c, "ModelInfo", func(response interface{}) {
		called = true
		result := response.(*params.UndertakerModelInfoResult)
		result.Result = params.UndertakerModelInfo{}
	})

	result, err := client.ModelInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
	c.Assert(result, gc.Equals, params.UndertakerModelInfoResult{})
}

func (s *UndertakerSuite) TestProcessDyingModel(c *gc.C) {
	var called bool
	client := s.mockClient(c, "ProcessDyingModel", func(response interface{}) {
		called = true
		c.Assert(response, gc.IsNil)
	})

	c.Assert(client.ProcessDyingModel(), jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *UndertakerSuite) TestRemoveModel(c *gc.C) {
	var called bool
	client := s.mockClient(c, "RemoveModel", func(response interface{}) {
		called = true
		c.Assert(response, gc.IsNil)
	})

	err := client.RemoveModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *UndertakerSuite) TestRemoveModelSecrets(c *gc.C) {
	var called bool
	client := s.mockClient(c, "RemoveModelSecrets", func(response interface{}) {
		called = true
		c.Assert(response, gc.IsNil)
	})

	err := client.RemoveModelSecrets()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *UndertakerSuite) mockClient(c *gc.C, expectedRequest string, callback func(response interface{})) *undertaker.Client {
	apiCaller := basetesting.APICallerFunc(func(
		objType string,
		version int,
		id, request string,
		args, response interface{},
	) error {
		c.Check(objType, gc.Equals, "Undertaker")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, expectedRequest)

		a, ok := args.(params.Entities)
		c.Check(ok, jc.IsTrue)
		c.Check(a.Entities, gc.DeepEquals, []params.Entity{{Tag: coretesting.ModelTag.String()}})

		callback(response)
		return nil
	})
	client, err := undertaker.NewClient(apiCaller, nil)
	c.Assert(err, jc.ErrorIsNil)
	return client
}

func (s *UndertakerSuite) TestWatchModelResourcesCreatesWatcher(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(
		objType string,
		version int,
		id, request string,
		args, response interface{},
	) error {
		c.Check(objType, gc.Equals, "Undertaker")
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "WatchModelResources")

		a, ok := args.(params.Entities)
		c.Check(ok, jc.IsTrue)
		c.Check(a.Entities, gc.DeepEquals, []params.Entity{{Tag: coretesting.ModelTag.String()}})

		resp, ok := response.(*params.NotifyWatchResults)
		c.Assert(ok, jc.IsTrue)
		resp.Results = []params.NotifyWatchResult{{
			NotifyWatcherId: "1001",
		}}
		return nil
	})

	expectWatcher := &fakeWatcher{}
	newWatcher := func(apiCaller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(apiCaller, gc.NotNil) // uncomparable
		c.Check(result, gc.Equals, params.NotifyWatchResult{
			NotifyWatcherId: "1001",
		})
		return expectWatcher
	}

	client, err := undertaker.NewClient(apiCaller, newWatcher)
	c.Assert(err, jc.ErrorIsNil)
	w, err := client.WatchModelResources()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(w, gc.Equals, expectWatcher)
}

func (s *UndertakerSuite) TestWatchModelResourcesError(c *gc.C) {
	var called bool
	client := s.mockClient(c, "WatchModelResources", func(response interface{}) {
		called = true
		_, ok := response.(*params.NotifyWatchResults)
		c.Check(ok, jc.IsTrue)
	})

	w, err := client.WatchModelResources()
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
	c.Assert(w, gc.IsNil)
	c.Assert(called, jc.IsTrue)
}

type fakeWatcher struct {
	watcher.NotifyWatcher
}
