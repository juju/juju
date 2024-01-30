// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type actionSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&actionSuite{})

func (s *actionSuite) TestAction(c *gc.C) {
	parallel := true
	group := "group"
	actionResult := params.ActionResult{
		Action: &params.Action{
			Name:           "backup",
			Parameters:     map[string]interface{}{"foo": "bar"},
			Parallel:       &parallel,
			ExecutionGroup: &group,
		},
	}
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "Actions")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "action-666"}}})
		c.Assert(result, gc.FitsTypeOf, &params.ActionResults{})
		*(result.(*params.ActionResults)) = params.ActionResults{
			Results: []params.ActionResult{actionResult},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	a, err := client.Action(context.Background(), names.NewActionTag("666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a.ID(), gc.Equals, "666")
	c.Assert(a.Name(), gc.Equals, actionResult.Action.Name)
	c.Assert(a.Params(), jc.DeepEquals, actionResult.Action.Parameters)
	c.Assert(a.Parallel(), jc.IsTrue)
	c.Assert(a.ExecutionGroup(), gc.Equals, "group")
}

func (s *actionSuite) TestActionError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "Actions")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "action-666"}}})
		c.Assert(result, gc.FitsTypeOf, &params.ActionResults{})
		*(result.(*params.ActionResults)) = params.ActionResults{
			Results: []params.ActionResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	_, err := client.Action(context.Background(), names.NewActionTag("666"))
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *actionSuite) TestActionBegin(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "BeginActions")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "action-666"}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "boom"}}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	err := client.ActionBegin(context.Background(), names.NewActionTag("666"))
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *actionSuite) TestActionFinish(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "FinishActions")
		c.Assert(arg, gc.DeepEquals, params.ActionExecutionResults{Results: []params.ActionExecutionResult{{
			ActionTag: "action-666",
			Status:    "failed",
			Results:   map[string]interface{}{"foo": "bar"},
			Message:   "oops",
		}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "boom"}}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	err := client.ActionFinish(context.Background(), names.NewActionTag("666"), "failed", map[string]interface{}{"foo": "bar"}, "oops")
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *actionSuite) TestActionStatus(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "ActionStatus")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "action-666"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{Result: "failed"}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	status, err := client.ActionStatus(context.Background(), names.NewActionTag("666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, "failed")
}

func (s *actionSuite) TestLogActionMessage(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "LogActionsMessages")
		c.Assert(arg, gc.DeepEquals, params.ActionMessageParams{
			Messages: []params.EntityString{{Tag: "action-666", Value: "hello"}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{&params.Error{Message: "biff"}}},
		}
		return nil
	})
	caller := basetesting.BestVersionCaller{apiCaller, 12}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.LogActionMessage(names.NewActionTag("666"), "hello")
	c.Assert(err, gc.ErrorMatches, "biff")
}

func (s *actionSuite) TestWatchActionNotifications(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "StringsWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "WatchActionNotifications")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				StringsWatcherId: "1",
				Changes:          []string{"666"},
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	w, err := unit.WatchActionNotifications()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange("666")
}

func (s *actionSuite) TestWatchActionNotificationsError(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.WatchActionNotifications()
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *actionSuite) TestWatchActionNotificationsErrorResults(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "WatchActionNotifications")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.WatchActionNotifications()
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *actionSuite) TestWatchActionNotificationsNoResults(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.WatchActionNotifications()
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
}

func (s *actionSuite) TestWatchActionNotificationsMoreResults(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, gc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{}, {}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.WatchActionNotifications()
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}
