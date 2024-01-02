// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type applicationSuite struct {
	coretesting.BaseSuite

	life      life.Value
	statusSet bool
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.life = life.Alive
}

func (s *applicationSuite) apiCallerFunc(c *gc.C) basetesting.APICallerFunc {
	return func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "NotifyWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, gc.Equals, "Uniter")
		switch request {
		case "Life":
			c.Assert(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-mysql"}}})
			c.Assert(result, gc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{
					Life: s.life,
				}},
			}
		case "Watch":
			c.Assert(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-mysql"}}})
			c.Assert(result, gc.FitsTypeOf, &params.NotifyWatchResults{})
			*(result.(*params.NotifyWatchResults)) = params.NotifyWatchResults{
				Results: []params.NotifyWatchResult{{
					NotifyWatcherId: "1",
				}},
			}
		case "CharmURL":
			c.Assert(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-mysql"}}})
			c.Assert(result, gc.FitsTypeOf, &params.StringBoolResults{})
			*(result.(*params.StringBoolResults)) = params.StringBoolResults{
				Results: []params.StringBoolResult{{
					Result: "ch:mysql",
					Ok:     true,
				}},
			}
		case "CharmModifiedVersion":
			c.Assert(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-mysql"}}})
			c.Assert(result, gc.FitsTypeOf, &params.IntResults{})
			*(result.(*params.IntResults)) = params.IntResults{
				Results: []params.IntResult{{
					Result: 1,
				}},
			}
		case "ApplicationStatus":
			c.Assert(arg, jc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
			c.Assert(result, gc.FitsTypeOf, &params.ApplicationStatusResults{})
			*(result.(*params.ApplicationStatusResults)) = params.ApplicationStatusResults{
				Results: []params.ApplicationStatusResult{{
					Application: params.StatusResult{Status: "alive"},
					Units: map[string]params.StatusResult{
						"unit-mysql-0": {Status: "dying"},
					},
				}},
			}
		case "SetApplicationStatus":
			c.Assert(arg, jc.DeepEquals, params.SetStatus{
				Entities: []params.EntityStatusArgs{
					{
						Tag:    "unit-mysql-0",
						Status: "blocked",
						Info:   "app blocked",
						Data:   map[string]interface{}{"foo": "bar"},
					},
				},
			})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
			s.statusSet = true
		default:
			c.Fatalf("unexpected api call %q", request)
		}
		return nil
	}
}

func (s *applicationSuite) TestNameTagAndString(c *gc.C) {
	client := uniter.NewState(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	tag := names.NewApplicationTag("mysql")
	app, err := client.Application(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.Name(), gc.Equals, "mysql")
	c.Assert(app.String(), gc.Equals, "mysql")
	c.Assert(app.Tag(), gc.Equals, tag)
	c.Assert(app.Life(), gc.Equals, life.Alive)
}

func (s *applicationSuite) TestWatch(c *gc.C) {
	client := uniter.NewState(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	app, err := client.Application(names.NewApplicationTag("mysql"))
	c.Assert(err, jc.ErrorIsNil)

	w, err := app.Watch()
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

func (s *applicationSuite) TestRefresh(c *gc.C) {
	client := uniter.NewState(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	app, err := client.Application(names.NewApplicationTag("mysql"))
	c.Assert(err, jc.ErrorIsNil)

	s.life = life.Dying
	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(app.Life(), gc.Equals, life.Dying)
}

func (s *applicationSuite) TestCharmURL(c *gc.C) {
	client := uniter.NewState(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	app, err := client.Application(names.NewApplicationTag("mysql"))
	c.Assert(err, jc.ErrorIsNil)

	curl, force, err := app.CharmURL()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.Equals, "ch:mysql")
	c.Assert(force, jc.IsTrue)
}

func (s *applicationSuite) TestCharmModifiedVersion(c *gc.C) {
	client := uniter.NewState(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	app, err := client.Application(names.NewApplicationTag("mysql"))
	c.Assert(err, jc.ErrorIsNil)

	ver, err := app.CharmModifiedVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ver, gc.Equals, 1)
}

func (s *applicationSuite) TestSetApplicationStatus(c *gc.C) {
	client := uniter.NewState(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	app, err := client.Application(names.NewApplicationTag("mysql"))
	c.Assert(err, jc.ErrorIsNil)

	err = app.SetStatus("mysql/0", status.Blocked, "app blocked", map[string]interface{}{"foo": "bar"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.statusSet, jc.IsTrue)
}

func (s *applicationSuite) TestApplicationStatus(c *gc.C) {
	client := uniter.NewState(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	app, err := client.Application(names.NewApplicationTag("mysql"))
	c.Assert(err, jc.ErrorIsNil)

	status, err := app.Status("mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, jc.DeepEquals, params.ApplicationStatusResult{
		Application: params.StatusResult{Status: "alive"},
		Units: map[string]params.StatusResult{
			"unit-mysql-0": {Status: "dying"},
		},
	})
}
