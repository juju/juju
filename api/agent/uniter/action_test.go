// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type actionSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&actionSuite{})

func (s *actionSuite) TestAction(c *tc.C) {
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
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "Actions")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "action-666"}}})
		c.Assert(result, tc.FitsTypeOf, &params.ActionResults{})
		*(result.(*params.ActionResults)) = params.ActionResults{
			Results: []params.ActionResult{actionResult},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	a, err := client.Action(context.Background(), names.NewActionTag("666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a.ID(), tc.Equals, "666")
	c.Assert(a.Name(), tc.Equals, actionResult.Action.Name)
	c.Assert(a.Params(), jc.DeepEquals, actionResult.Action.Parameters)
	c.Assert(a.Parallel(), jc.IsTrue)
	c.Assert(a.ExecutionGroup(), tc.Equals, "group")
}

func (s *actionSuite) TestActionError(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "Actions")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "action-666"}}})
		c.Assert(result, tc.FitsTypeOf, &params.ActionResults{})
		*(result.(*params.ActionResults)) = params.ActionResults{
			Results: []params.ActionResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	_, err := client.Action(context.Background(), names.NewActionTag("666"))
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *actionSuite) TestActionBegin(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "BeginActions")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "action-666"}}})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "boom"}}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	err := client.ActionBegin(context.Background(), names.NewActionTag("666"))
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *actionSuite) TestActionFinish(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "FinishActions")
		c.Assert(arg, tc.DeepEquals, params.ActionExecutionResults{Results: []params.ActionExecutionResult{{
			ActionTag: "action-666",
			Status:    "failed",
			Results:   map[string]interface{}{"foo": "bar"},
			Message:   "oops",
		}}})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "boom"}}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	err := client.ActionFinish(context.Background(), names.NewActionTag("666"), "failed", map[string]interface{}{"foo": "bar"}, "oops")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *actionSuite) TestActionStatus(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "ActionStatus")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "action-666"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{Result: "failed"}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	status, err := client.ActionStatus(context.Background(), names.NewActionTag("666"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, tc.Equals, "failed")
}

func (s *actionSuite) TestLogActionMessage(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "LogActionsMessages")
		c.Assert(arg, tc.DeepEquals, params.ActionMessageParams{
			Messages: []params.EntityString{{Tag: "action-666", Value: "hello"}},
		})
		c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{&params.Error{Message: "biff"}}},
		}
		return nil
	})
	caller := basetesting.BestVersionCaller{apiCaller, 12}
	client := uniter.NewClient(caller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.LogActionMessage(context.Background(), names.NewActionTag("666"), "hello")
	c.Assert(err, tc.ErrorMatches, "biff")
}

func (s *actionSuite) TestWatchActionNotifications(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "StringsWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "WatchActionNotifications")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
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
	w, err := unit.WatchActionNotifications(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange("666")
}

func (s *actionSuite) TestWatchActionNotificationsError(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return errors.New("boom")
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.WatchActionNotifications(context.Background())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *actionSuite) TestWatchActionNotificationsErrorResults(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, tc.Equals, "Uniter")
		c.Assert(request, tc.Equals, "WatchActionNotifications")
		c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{
				Error: &params.Error{Message: "boom"},
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.WatchActionNotifications(context.Background())
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *actionSuite) TestWatchActionNotificationsNoResults(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.WatchActionNotifications(context.Background())
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 0")
}

func (s *actionSuite) TestWatchActionNotificationsMoreResults(c *tc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(result, tc.FitsTypeOf, &params.StringsWatchResults{})
		*(result.(*params.StringsWatchResults)) = params.StringsWatchResults{
			Results: []params.StringsWatchResult{{}, {}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.WatchActionNotifications(context.Background())
	c.Assert(err, tc.ErrorMatches, "expected 1 result, got 2")
}
