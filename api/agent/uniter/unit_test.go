// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/charm"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type unitSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&unitSuite{})

func (s *unitSuite) TestUnitAndUnitTag(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "Refresh")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.UnitRefreshResults{})
		*(result.(*params.UnitRefreshResults)) = params.UnitRefreshResults{
			Results: []params.UnitRefreshResult{{
				Life: life.Alive,
			}},
		}
		return nil
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)
	unit, err := client.Unit(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/0")
	c.Assert(unit.Tag(), gc.Equals, tag)
	c.Assert(unit.Life(), gc.Equals, life.Alive)
	c.Assert(unit.ApplicationName(), gc.Equals, "mysql")
	c.Assert(unit.ApplicationTag(), gc.Equals, names.NewApplicationTag("mysql"))
}

func (s *unitSuite) TestUnitAndUnitTagNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)
	_, err := client.Unit(context.Background(), tag)
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestSetAgentStatus(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "SetAgentStatus")
		c.Assert(arg, gc.DeepEquals, params.SetStatus{
			Entities: []params.EntityStatusArgs{
				{Tag: "unit-mysql-0", Status: "idle", Info: "blah", Data: map[string]interface{}{"foo": "bar"}},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetAgentStatus(context.Background(), status.Idle, "blah", map[string]interface{}{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, "biff")
}

func (s *unitSuite) TestSetAgentStatusNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetAgentStatus(context.Background(), status.Idle, "blah", map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestSetUnitStatus(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "SetUnitStatus")
		c.Assert(arg, gc.DeepEquals, params.SetStatus{
			Entities: []params.EntityStatusArgs{
				{Tag: "unit-mysql-0", Status: "idle", Info: "blah", Data: map[string]interface{}{"foo": "bar"}},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetUnitStatus(context.Background(), status.Idle, "blah", map[string]interface{}{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, "biff")
}

func (s *unitSuite) TestSetUnitStatusNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetUnitStatus(context.Background(), status.Idle, "blah", map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestUnitStatus(c *gc.C) {
	now := time.Now()
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "UnitStatus")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StatusResults{})
		*(result.(*params.StatusResults)) = params.StatusResults{
			Results: []params.StatusResult{{
				Id:     "mysql/0",
				Life:   life.Alive,
				Status: "maintenance",
				Info:   "blah",
				Data:   map[string]interface{}{"foo": "bar"},
				Since:  &now,
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	statusInfo, err := unit.UnitStatus(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo, gc.DeepEquals, params.StatusResult{
		Id:     "mysql/0",
		Life:   life.Alive,
		Status: status.Maintenance.String(),
		Info:   "blah",
		Data:   map[string]interface{}{"foo": "bar"},
		Since:  &now,
	})
}

func (s *unitSuite) TestUnitStatusNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.UnitStatus(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestEnsureDead(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "EnsureDead")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.EnsureDead(context.Background())
	c.Assert(err, gc.ErrorMatches, "biff")
}

func (s *unitSuite) TestEnsureDeadNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.EnsureDead(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestDestroy(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "Destroy")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.Destroy(context.Background())
	c.Assert(err, gc.ErrorMatches, "biff")
}

func (s *unitSuite) TestDestroyNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.Destroy(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestDestroyAllSubordinates(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "DestroyAllSubordinates")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.DestroyAllSubordinates(context.Background())
	c.Assert(err, gc.ErrorMatches, "biff")
}

func (s *unitSuite) TestDestroyAllSubordinatesNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.DestroyAllSubordinates(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestRefresh(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "Refresh")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.UnitRefreshResults{})
		*(result.(*params.UnitRefreshResults)) = params.UnitRefreshResults{
			Results: []params.UnitRefreshResult{{
				Life:       life.Dying,
				Resolved:   params.ResolvedRetryHooks,
				ProviderID: "666",
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.Refresh(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unit.Life(), gc.Equals, life.Dying)
	c.Assert(unit.Resolved(), gc.Equals, params.ResolvedRetryHooks)
	c.Assert(unit.Life(), gc.Equals, life.Dying)
}

func (s *unitSuite) TestRefreshNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.Refresh(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestClearResolved(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "ClearResolved")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.ClearResolved(context.Background())
	c.Assert(err, gc.ErrorMatches, "biff")
}

func (s *unitSuite) TestClearResolvedNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.ClearResolved(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatch(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "NotifyWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "WatchUnit")
		c.Assert(arg, gc.DeepEquals, params.Entity{Tag: "unit-mysql-0"})
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResult{})
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			NotifyWatcherId: "1",
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	w, err := unit.Watch(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
}

func (s *unitSuite) TestWatchNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.Watch(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatchRelations(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "StringsWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "WatchUnitRelations")
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
	w, err := unit.WatchRelations(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange("666")
}

func (s *unitSuite) TestWatchRelationsNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.WatchRelations(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestAssignedMachine(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "AssignedMachine")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "machine-666",
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	tag, err := unit.AssignedMachine(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tag, gc.Equals, names.NewMachineTag("666"))
}

func (s *unitSuite) TestAssignedMachineNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.AssignedMachine(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestPrincipalName(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "GetPrincipal")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringBoolResults{})
		*(result.(*params.StringBoolResults)) = params.StringBoolResults{
			Results: []params.StringBoolResult{{
				Result: "unit-wordpress-0",
				Ok:     true,
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	name, ok, err := unit.PrincipalName(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "wordpress/0")
	c.Assert(ok, jc.IsTrue)
}

func (s *unitSuite) TestPrincipalNameNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, _, err := unit.PrincipalName(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestHasSubordinates(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "HasSubordinates")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.BoolResults{})
		*(result.(*params.BoolResults)) = params.BoolResults{
			Results: []params.BoolResult{{
				Result: true,
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	ok, err := unit.HasSubordinates(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsTrue)
}

func (s *unitSuite) TestHasSubordinatesNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.HasSubordinates(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestPublicAddress(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "PublicAddress")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "1.1.1.1",
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	address, err := unit.PublicAddress(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "1.1.1.1")
}

func (s *unitSuite) TestPublicAddressNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.PublicAddress(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestPrivateAddress(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "PrivateAddress")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "1.1.1.1",
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	address, err := unit.PrivateAddress(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "1.1.1.1")
}

func (s *unitSuite) TestPrivateAddressNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.PrivateAddress(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestAvailabilityZone(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "AvailabilityZone")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "a-zone",
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	address, err := unit.AvailabilityZone(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "a-zone")
}

func (s *unitSuite) TestAvailabilityZoneNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.AvailabilityZone(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestCharmURL(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "CharmURL")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringBoolResults{})
		*(result.(*params.StringBoolResults)) = params.StringBoolResults{
			Results: []params.StringBoolResult{{
				Result: "ch:mysql",
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	curl, err := unit.CharmURL(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.Equals, "ch:mysql")
}

func (s *unitSuite) TestCharmURLNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	_, err := unit.CharmURL(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestSetCharmURL(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "SetCharmURL")
		c.Assert(arg, gc.DeepEquals, params.EntitiesCharmURL{
			Entities: []params.EntityCharmURL{
				{Tag: "unit-mysql-0", CharmURL: "ch:mysql"},
			},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetCharmURL(context.Background(), "ch:mysql")
	c.Assert(err, gc.ErrorMatches, "biff")
}

func (s *unitSuite) TestSetCharmURLNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetCharmURL(context.Background(), "ch:mysql")
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestNetworkInfo(c *gc.C) {
	relId := 2
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Check(objType, gc.Equals, "Uniter")
		c.Check(version, gc.Equals, 0)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "NetworkInfo")
		c.Check(arg, gc.DeepEquals, params.NetworkInfoParams{
			Unit:       "unit-mysql-0",
			Endpoints:  []string{"server"},
			RelationId: &relId,
		})
		c.Assert(result, gc.FitsTypeOf, &params.NetworkInfoResults{})
		*(result.(*params.NetworkInfoResults)) = params.NetworkInfoResults{
			Results: map[string]params.NetworkInfoResult{
				"db": {
					Error: &params.Error{Message: "FAIL"},
				}},
		}
		return nil
	})

	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	result, err := unit.NetworkInfo(context.Background(), []string{"server"}, &relId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result["db"].Error, gc.ErrorMatches, "FAIL")
}

func (s *unitSuite) TestNetworkInfoNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	relId := 2
	_, err := unit.NetworkInfo(context.Background(), []string{"server"}, &relId)
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestConfigSettings(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "ConfigSettings")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.ConfigSettingsResults{})
		*(result.(*params.ConfigSettingsResults)) = params.ConfigSettingsResults{
			Results: []params.ConfigSettingsResult{{
				Settings: params.ConfigSettings{"foo": "bar"},
			}},
		}
		return nil
	})

	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	settings, err := unit.ConfigSettings(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"foo": "bar",
	})
}

func (s *unitSuite) TestConfigSettingsNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.ConfigSettings(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatchConfigSettingsHash(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "StringsWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "WatchConfigSettingsHash")
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
	w, err := unit.WatchConfigSettingsHash(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange("666")
}

func (s *unitSuite) TestWatchConfigSettingsHashNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.WatchConfigSettingsHash(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatchTrustConfigSettingsHash(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "StringsWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "WatchTrustConfigSettingsHash")
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
	w, err := unit.WatchTrustConfigSettingsHash(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange("666")
}

func (s *unitSuite) TestWatchTrustConfigSettingsHashNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.WatchTrustConfigSettingsHash(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatchAddressesHash(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "StringsWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "WatchUnitAddressesHash")
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
	w, err := unit.WatchAddressesHash(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange("666")
}

func (s *unitSuite) TestWatchAddressesHashNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.WatchAddressesHash(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestRelationStatus(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "RelationsStatus")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.RelationUnitStatusResults{})
		*(result.(*params.RelationUnitStatusResults)) = params.RelationUnitStatusResults{
			Results: []params.RelationUnitStatusResult{{
				RelationResults: []params.RelationUnitStatus{{
					RelationTag: "relation-wordpress.server#mysql.db",
					Suspended:   true,
					InScope:     true,
				}},
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	relStatus, err := unit.RelationsStatus(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relStatus, jc.DeepEquals, []uniter.RelationStatus{{
		Tag:       names.NewRelationTag("wordpress:server mysql:db"),
		Suspended: true,
		InScope:   true,
	}})
}

func (s *unitSuite) TestRelationStatusNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.RelationsStatus(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestUnitState(c *gc.C) {
	unitState := params.UnitStateResult{
		StorageState:  "storage",
		SecretState:   "secret",
		UniterState:   "uniter",
		CharmState:    map[string]string{"foo": "bar"},
		RelationState: map[int]string{666: "666"},
	}
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "State")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.UnitStateResults{})
		*(result.(*params.UnitStateResults)) = params.UnitStateResults{
			Results: []params.UnitStateResult{unitState},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	result, err := unit.State(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, unitState)
}

func (s *unitSuite) TestUnitStateNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.State(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestSetState(c *gc.C) {
	unitState := params.SetUnitStateArg{
		Tag:           "unit-mysql-0",
		CharmState:    &map[string]string{"foo": "bar"},
		RelationState: &map[int]string{666: "666"},
	}
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "SetState")
		c.Assert(arg, gc.DeepEquals, params.SetUnitStateArgs{
			Args: []params.SetUnitStateArg{unitState},
		})
		c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
		*(result.(*params.ErrorResults)) = params.ErrorResults{
			Results: []params.ErrorResult{{Error: &params.Error{Message: "biff"}}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	err := unit.SetState(context.Background(), unitState)
	c.Assert(err, gc.ErrorMatches, "biff")
}

func (s *unitSuite) TestSetStateNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	err := unit.SetState(context.Background(), params.SetUnitStateArg{})
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestWatchInstanceData(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "NotifyWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "WatchInstanceData")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
		*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
			Results: []params.NotifyWatchResult{{
				NotifyWatcherId: "1",
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	w, err := unit.WatchInstanceData(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
}

func (s *unitSuite) TestWatchInstanceDataNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.WatchInstanceData(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestLXDProfileName(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "LXDProfileName")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			Results: []params.StringResult{{
				Result: "juju-default-mysql-0",
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	profile, err := unit.LXDProfileName(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(profile, gc.Equals, "juju-default-mysql-0")
}

func (s *unitSuite) TestLXDProfileNameNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.LXDProfileName(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *unitSuite) TestCanApplyLXDProfile(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "Uniter")
		c.Assert(request, gc.Equals, "CanApplyLXDProfile")
		c.Assert(arg, gc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
		c.Assert(result, gc.FitsTypeOf, &params.BoolResults{})
		*(result.(*params.BoolResults)) = params.BoolResults{
			Results: []params.BoolResult{{
				Result: true,
			}},
		}
		return nil
	})
	client := uniter.NewClient(apiCaller, names.NewUnitTag("mysql/0"))
	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))
	canApply, err := unit.CanApplyLXDProfile(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(canApply, jc.IsTrue)
}

func (s *unitSuite) TestCanApplyLXDProfileNotImplemented(c *gc.C) {
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		return apiservererrors.ServerError(errors.NotImplementedf("not implemented"))
	})
	tag := names.NewUnitTag("mysql/0")
	client := uniter.NewClient(apiCaller, tag)

	unit := uniter.CreateUnit(client, names.NewUnitTag("mysql/0"))

	_, err := unit.CanApplyLXDProfile(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}
