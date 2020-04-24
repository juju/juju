// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/lifeflag"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
)

type FacadeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FacadeSuite{})

func (*FacadeSuite) TestLifeCall(c *gc.C) {
	var called bool
	caller := apiCaller(c, func(request string, args, _ interface{}) error {
		called = true
		c.Check(request, gc.Equals, "Life")
		c.Check(args, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "application-blah"}},
		})
		return nil
	})
	facade := lifeflag.NewFacade(caller, nil)

	facade.Life(names.NewApplicationTag("blah"))
	c.Check(called, jc.IsTrue)
}

func (*FacadeSuite) TestLifeCallError(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		return errors.New("crunch belch")
	})
	facade := lifeflag.NewFacade(caller, nil)

	result, err := facade.Life(names.NewApplicationTag("blah"))
	c.Check(err, gc.ErrorMatches, "crunch belch")
	c.Check(result, gc.Equals, life.Value(""))
}

func (*FacadeSuite) TestLifeNoResultsError(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		return nil
	})
	facade := lifeflag.NewFacade(caller, nil)

	result, err := facade.Life(names.NewApplicationTag("blah"))
	c.Check(err, gc.ErrorMatches, "expected 1 Life result, got 0")
	c.Check(result, gc.Equals, life.Value(""))
}

func (*FacadeSuite) TestLifeExtraResultsError(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.LifeResults)
		c.Assert(ok, jc.IsTrue)
		*typed = params.LifeResults{
			Results: make([]params.LifeResult, 2),
		}
		return nil
	})
	facade := lifeflag.NewFacade(caller, nil)

	result, err := facade.Life(names.NewApplicationTag("blah"))
	c.Check(err, gc.ErrorMatches, "expected 1 Life result, got 2")
	c.Check(result, gc.Equals, life.Value(""))
}

func (*FacadeSuite) TestLifeNotFoundError(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.LifeResults)
		c.Assert(ok, jc.IsTrue)
		*typed = params.LifeResults{
			Results: []params.LifeResult{{
				Error: &params.Error{Code: params.CodeNotFound},
			}},
		}
		return nil
	})
	facade := lifeflag.NewFacade(caller, nil)

	result, err := facade.Life(names.NewApplicationTag("blah"))
	c.Check(err, gc.Equals, lifeflag.ErrNotFound)
	c.Check(result, gc.Equals, life.Value(""))
}

func (*FacadeSuite) TestLifeInvalidResultError(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.LifeResults)
		c.Assert(ok, jc.IsTrue)
		*typed = params.LifeResults{
			Results: []params.LifeResult{{Life: "decomposed"}},
		}
		return nil
	})
	facade := lifeflag.NewFacade(caller, nil)

	result, err := facade.Life(names.NewApplicationTag("blah"))
	c.Check(err, gc.ErrorMatches, `life value "decomposed" not valid`)
	c.Check(result, gc.Equals, life.Value(""))
}

func (*FacadeSuite) TestLifeSuccess(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.LifeResults)
		c.Assert(ok, jc.IsTrue)
		*typed = params.LifeResults{
			Results: []params.LifeResult{{Life: "dying"}},
		}
		return nil
	})
	facade := lifeflag.NewFacade(caller, nil)

	result, err := facade.Life(names.NewApplicationTag("blah"))
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.Equals, life.Dying)
}

func (*FacadeSuite) TestWatchCall(c *gc.C) {
	var called bool
	caller := apiCaller(c, func(request string, args, _ interface{}) error {
		called = true
		c.Check(request, gc.Equals, "Watch")
		c.Check(args, jc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "application-blah"}},
		})
		return nil
	})
	facade := lifeflag.NewFacade(caller, nil)

	facade.Watch(names.NewApplicationTag("blah"))
	c.Check(called, jc.IsTrue)
}

func (*FacadeSuite) TestWatchCallError(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		return errors.New("crunch belch")
	})
	facade := lifeflag.NewFacade(caller, nil)

	watcher, err := facade.Watch(names.NewApplicationTag("blah"))
	c.Check(err, gc.ErrorMatches, "crunch belch")
	c.Check(watcher, gc.IsNil)
}

func (*FacadeSuite) TestWatchNoResultsError(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		return nil
	})
	facade := lifeflag.NewFacade(caller, nil)

	watcher, err := facade.Watch(names.NewApplicationTag("blah"))
	c.Check(err, gc.ErrorMatches, "expected 1 Watch result, got 0")
	c.Check(watcher, gc.IsNil)
}

func (*FacadeSuite) TestWatchExtraResultsError(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.NotifyWatchResults)
		c.Assert(ok, jc.IsTrue)
		*typed = params.NotifyWatchResults{
			Results: make([]params.NotifyWatchResult, 2),
		}
		return nil
	})
	facade := lifeflag.NewFacade(caller, nil)

	watcher, err := facade.Watch(names.NewApplicationTag("blah"))
	c.Check(err, gc.ErrorMatches, "expected 1 Watch result, got 2")
	c.Check(watcher, gc.IsNil)
}

func (*FacadeSuite) TestWatchNotFoundError(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.NotifyWatchResults)
		c.Assert(ok, jc.IsTrue)
		*typed = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Code: params.CodeNotFound},
			}},
		}
		return nil
	})
	facade := lifeflag.NewFacade(caller, nil)

	watcher, err := facade.Watch(names.NewApplicationTag("blah"))
	c.Check(err, gc.Equals, lifeflag.ErrNotFound)
	c.Check(watcher, gc.IsNil)
}

func (*FacadeSuite) TestWatchSuccess(c *gc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.NotifyWatchResults)
		c.Assert(ok, jc.IsTrue)
		*typed = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				NotifyWatcherId: "123",
			}},
		}
		return nil
	})
	expectWatcher := &struct{ watcher.NotifyWatcher }{}
	newWatcher := func(apiCaller base.APICaller, result params.NotifyWatchResult) watcher.NotifyWatcher {
		c.Check(apiCaller, gc.NotNil) // uncomparable
		c.Check(result, jc.DeepEquals, params.NotifyWatchResult{
			NotifyWatcherId: "123",
		})
		return expectWatcher
	}
	facade := lifeflag.NewFacade(caller, newWatcher)

	watcher, err := facade.Watch(names.NewApplicationTag("blah"))
	c.Check(err, jc.ErrorIsNil)
	c.Check(watcher, gc.Equals, expectWatcher)
}

func apiCaller(c *gc.C, check func(request string, arg, result interface{}) error) base.APICaller {
	return apitesting.APICallerFunc(func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, gc.Equals, "LifeFlag")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		return check(request, arg, result)
	})
}
