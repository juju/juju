// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/applicationscaler"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

type APISuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&APISuite{})

func (s *APISuite) TestRescaleMethodName(c *gc.C) {
	var called bool
	caller := apiCaller(c, func(request string, _, _ interface{}) error {
		called = true
		c.Check(request, gc.Equals, "Rescale")
		return nil
	})
	api := applicationscaler.NewAPI(caller, nil)

	api.Rescale(nil)
	c.Check(called, jc.IsTrue)
}

func (s *APISuite) TestRescaleBadArgs(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		panic("should not be called")
	})
	api := applicationscaler.NewAPI(caller, nil)

	err := api.Rescale([]string{"good-name", "bad/name"})
	c.Check(err, gc.ErrorMatches, `application name "bad/name" not valid`)
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

func (s *APISuite) TestRescaleConvertArgs(c *gc.C) {
	var called bool
	caller := apiCaller(c, func(_ string, arg, _ interface{}) error {
		called = true
		c.Check(arg, gc.DeepEquals, params.Entities{
			Entities: []params.Entity{{
				"application-foo",
			}, {
				"application-bar-baz",
			}},
		})
		return nil
	})
	api := applicationscaler.NewAPI(caller, nil)

	api.Rescale([]string{"foo", "bar-baz"})
	c.Check(called, jc.IsTrue)
}

func (s *APISuite) TestRescaleCallError(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		return errors.New("snorble flip")
	})
	api := applicationscaler.NewAPI(caller, nil)

	err := api.Rescale(nil)
	c.Check(err, gc.ErrorMatches, "snorble flip")
}

func (s *APISuite) TestRescaleFirstError(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, result interface{}) error {
		resultPtr, ok := result.(*params.ErrorResults)
		c.Assert(ok, jc.IsTrue)
		*resultPtr = params.ErrorResults{Results: []params.ErrorResult{{
			nil,
		}, {
			&params.Error{Message: "expect this error"},
		}, {
			&params.Error{Message: "not this one"},
		}, {
			nil,
		}}}
		return nil
	})
	api := applicationscaler.NewAPI(caller, nil)

	err := api.Rescale(nil)
	c.Check(err, gc.ErrorMatches, "expect this error")
}

func (s *APISuite) TestRescaleNoError(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		return nil
	})
	api := applicationscaler.NewAPI(caller, nil)

	err := api.Rescale(nil)
	c.Check(err, jc.ErrorIsNil)
}

func (s *APISuite) TestWatchMethodName(c *gc.C) {
	var called bool
	caller := apiCaller(c, func(request string, _, _ interface{}) error {
		called = true
		c.Check(request, gc.Equals, "Watch")
		return errors.New("irrelevant")
	})
	api := applicationscaler.NewAPI(caller, nil)

	api.Watch()
	c.Check(called, jc.IsTrue)
}

func (s *APISuite) TestWatchError(c *gc.C) {
	var called bool
	caller := apiCaller(c, func(request string, _, _ interface{}) error {
		called = true
		c.Check(request, gc.Equals, "Watch")
		return errors.New("blam pow")
	})
	api := applicationscaler.NewAPI(caller, nil)

	watcher, err := api.Watch()
	c.Check(watcher, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "blam pow")
	c.Check(called, jc.IsTrue)
}

func (s *APISuite) TestWatchSuccess(c *gc.C) {
	expectResult := params.StringsWatchResult{
		StringsWatcherId: "123",
		Changes:          []string{"ping", "pong", "pung"},
	}
	caller := apiCaller(c, func(_ string, _, result interface{}) error {
		resultPtr, ok := result.(*params.StringsWatchResult)
		c.Assert(ok, jc.IsTrue)
		*resultPtr = expectResult
		return nil
	})
	expectWatcher := &stubWatcher{}
	newWatcher := func(gotCaller base.APICaller, gotResult params.StringsWatchResult) watcher.StringsWatcher {
		c.Check(gotCaller, gc.NotNil) // uncomparable
		c.Check(gotResult, jc.DeepEquals, expectResult)
		return expectWatcher
	}
	api := applicationscaler.NewAPI(caller, newWatcher)

	watcher, err := api.Watch()
	c.Check(watcher, gc.Equals, expectWatcher)
	c.Check(err, jc.ErrorIsNil)
}

func apiCaller(c *gc.C, check func(request string, arg, result interface{}) error) base.APICaller {
	return apitesting.APICallerFunc(func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, gc.Equals, "ApplicationScaler")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		return check(request, arg, result)
	})
}

type stubWatcher struct {
	watcher.StringsWatcher
}
