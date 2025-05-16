// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/common/lifeflag"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker"
	"github.com/juju/juju/rpc/params"
)

type FacadeSuite struct {
	testhelpers.IsolationSuite
}

func TestFacadeSuite(t *stdtesting.T) { tc.Run(t, &FacadeSuite{}) }
func (*FacadeSuite) TestLifeCall(c *tc.C) {
	var called bool
	caller := apiCaller(c, func(request string, args, _ interface{}) error {
		called = true
		c.Check(request, tc.Equals, "Life")
		c.Check(args, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "application-blah"}},
		})
		return nil
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")

	facade.Life(c.Context(), names.NewApplicationTag("blah"))
	c.Check(called, tc.IsTrue)
}

func (*FacadeSuite) TestLifeCallError(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		return errors.New("crunch belch")
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")

	result, err := facade.Life(c.Context(), names.NewApplicationTag("blah"))
	c.Check(err, tc.ErrorMatches, "crunch belch")
	c.Check(result, tc.Equals, life.Value(""))
}

func (*FacadeSuite) TestLifeNoResultsError(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		return nil
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")

	result, err := facade.Life(c.Context(), names.NewApplicationTag("blah"))
	c.Check(err, tc.ErrorMatches, "expected 1 Life result, got 0")
	c.Check(result, tc.Equals, life.Value(""))
}

func (*FacadeSuite) TestLifeExtraResultsError(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.LifeResults)
		c.Assert(ok, tc.IsTrue)
		*typed = params.LifeResults{
			Results: make([]params.LifeResult, 2),
		}
		return nil
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")

	result, err := facade.Life(c.Context(), names.NewApplicationTag("blah"))
	c.Check(err, tc.ErrorMatches, "expected 1 Life result, got 2")
	c.Check(result, tc.Equals, life.Value(""))
}

func (*FacadeSuite) TestLifeNotFoundError(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.LifeResults)
		c.Assert(ok, tc.IsTrue)
		*typed = params.LifeResults{
			Results: []params.LifeResult{{
				Error: &params.Error{Code: params.CodeNotFound},
			}},
		}
		return nil
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")

	result, err := facade.Life(c.Context(), names.NewApplicationTag("blah"))
	c.Check(err, tc.Equals, lifeflag.ErrEntityNotFound)
	c.Check(result, tc.Equals, life.Value(""))
}

func (*FacadeSuite) TestLifeInvalidResultError(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.LifeResults)
		c.Assert(ok, tc.IsTrue)
		*typed = params.LifeResults{
			Results: []params.LifeResult{{Life: "decomposed"}},
		}
		return nil
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")

	result, err := facade.Life(c.Context(), names.NewApplicationTag("blah"))
	c.Check(err, tc.ErrorMatches, `life value "decomposed" not valid`)
	c.Check(result, tc.Equals, life.Value(""))
}

func (*FacadeSuite) TestLifeSuccess(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.LifeResults)
		c.Assert(ok, tc.IsTrue)
		*typed = params.LifeResults{
			Results: []params.LifeResult{{Life: "dying"}},
		}
		return nil
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")

	result, err := facade.Life(c.Context(), names.NewApplicationTag("blah"))
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, life.Dying)
}

func (*FacadeSuite) TestWatchCall(c *tc.C) {
	var called bool
	caller := apiCaller(c, func(request string, args, _ interface{}) error {
		called = true
		c.Check(request, tc.Equals, "Watch")
		c.Check(args, tc.DeepEquals, params.Entities{
			Entities: []params.Entity{{Tag: "application-blah"}},
		})
		return nil
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")

	facade.Watch(c.Context(), names.NewApplicationTag("blah"))
	c.Check(called, tc.IsTrue)
}

func (*FacadeSuite) TestWatchCallError(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		return errors.New("crunch belch")
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")

	watcher, err := facade.Watch(c.Context(), names.NewApplicationTag("blah"))
	c.Check(err, tc.ErrorMatches, "crunch belch")
	c.Check(watcher, tc.IsNil)
}

func (*FacadeSuite) TestWatchNoResultsError(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, _ interface{}) error {
		return nil
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")

	watcher, err := facade.Watch(c.Context(), names.NewApplicationTag("blah"))
	c.Check(err, tc.ErrorMatches, "expected 1 Watch result, got 0")
	c.Check(watcher, tc.IsNil)
}

func (*FacadeSuite) TestWatchExtraResultsError(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.NotifyWatchResults)
		c.Assert(ok, tc.IsTrue)
		*typed = params.NotifyWatchResults{
			Results: make([]params.NotifyWatchResult, 2),
		}
		return nil
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")

	watcher, err := facade.Watch(c.Context(), names.NewApplicationTag("blah"))
	c.Check(err, tc.ErrorMatches, "expected 1 Watch result, got 2")
	c.Check(watcher, tc.IsNil)
}

func (*FacadeSuite) TestWatchNotFoundError(c *tc.C) {
	caller := apiCaller(c, func(_ string, _, results interface{}) error {
		typed, ok := results.(*params.NotifyWatchResults)
		c.Assert(ok, tc.IsTrue)
		*typed = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				Error: &params.Error{Code: params.CodeNotFound},
			}},
		}
		return nil
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")

	watcher, err := facade.Watch(c.Context(), names.NewApplicationTag("blah"))
	c.Check(err, tc.Equals, lifeflag.ErrEntityNotFound)
	c.Check(watcher, tc.IsNil)
}

func (*FacadeSuite) TestWatchSuccess(c *tc.C) {
	caller := apitesting.APICallerFunc(func(facade string, version int, id, request string, arg, result interface{}) error {
		switch facade {
		case "LifeFlag":
			c.Check(request, tc.Equals, "Watch")
			c.Check(version, tc.Equals, 0)
			c.Check(id, tc.Equals, "")
			typed, ok := result.(*params.NotifyWatchResults)
			c.Assert(ok, tc.IsTrue)
			*typed = params.NotifyWatchResults{
				Results: []params.NotifyWatchResult{{
					NotifyWatcherId: "123",
				}},
			}
			return nil
		case "NotifyWatcher":
			return worker.ErrKilled
		default:
			c.Fatalf("unknown facade %q", facade)
			return nil
		}
	})
	facade := lifeflag.NewClient(caller, "LifeFlag")
	watcher, err := facade.Watch(c.Context(), names.NewApplicationTag("blah"))
	c.Check(err, tc.ErrorIsNil)
	workertest.CheckKilled(c, watcher)
}

func apiCaller(c *tc.C, check func(request string, arg, result interface{}) error) base.APICaller {
	return apitesting.APICallerFunc(func(facade string, version int, id, request string, arg, result interface{}) error {
		c.Check(facade, tc.Equals, "LifeFlag")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		return check(request, arg, result)
	})
}
