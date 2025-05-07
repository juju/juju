// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type applicationSuite struct {
	coretesting.BaseSuite

	life      life.Value
	statusSet bool
}

var _ = tc.Suite(&applicationSuite{})

func (s *applicationSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.life = life.Alive
}

func (s *applicationSuite) apiCallerFunc(c *tc.C) basetesting.APICallerFunc {
	return func(objType string, version int, id, request string, arg, result interface{}) error {
		if objType == "NotifyWatcher" {
			if request != "Next" && request != "Stop" {
				c.Fatalf("unexpected watcher request %q", request)
			}
			return nil
		}
		c.Assert(objType, tc.Equals, "Uniter")
		switch request {
		case "Life":
			c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-mysql"}}})
			c.Assert(result, tc.FitsTypeOf, &params.LifeResults{})
			*(result.(*params.LifeResults)) = params.LifeResults{
				Results: []params.LifeResult{{
					Life: s.life,
				}},
			}
		case "WatchApplication":
			c.Assert(arg, tc.DeepEquals, params.Entity{Tag: "application-mysql"})
			c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResult{})
			*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
				NotifyWatcherId: "1",
			}
		case "CharmURL":
			c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-mysql"}}})
			c.Assert(result, tc.FitsTypeOf, &params.StringBoolResults{})
			*(result.(*params.StringBoolResults)) = params.StringBoolResults{
				Results: []params.StringBoolResult{{
					Result: "ch:mysql",
					Ok:     true,
				}},
			}
		case "CharmModifiedVersion":
			c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "application-mysql"}}})
			c.Assert(result, tc.FitsTypeOf, &params.IntResults{})
			*(result.(*params.IntResults)) = params.IntResults{
				Results: []params.IntResult{{
					Result: 1,
				}},
			}
		case "ApplicationStatus":
			c.Assert(arg, tc.DeepEquals, params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}})
			c.Assert(result, tc.FitsTypeOf, &params.ApplicationStatusResults{})
			*(result.(*params.ApplicationStatusResults)) = params.ApplicationStatusResults{
				Results: []params.ApplicationStatusResult{{
					Application: params.StatusResult{Status: "alive"},
					Units: map[string]params.StatusResult{
						"unit-mysql-0": {Status: "dying"},
					},
				}},
			}
		case "SetApplicationStatus":
			c.Assert(arg, tc.DeepEquals, params.SetStatus{
				Entities: []params.EntityStatusArgs{
					{
						Tag:    "unit-mysql-0",
						Status: "blocked",
						Info:   "app blocked",
						Data:   map[string]interface{}{"foo": "bar"},
					},
				},
			})
			c.Assert(result, tc.FitsTypeOf, &params.ErrorResults{})
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

func (s *applicationSuite) TestNameTagAndString(c *tc.C) {
	client := uniter.NewClient(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	tag := names.NewApplicationTag("mysql")
	app, err := client.Application(context.Background(), tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(app.Name(), tc.Equals, "mysql")
	c.Assert(app.String(), tc.Equals, "mysql")
	c.Assert(app.Tag(), tc.Equals, tag)
	c.Assert(app.Life(), tc.Equals, life.Alive)
}

func (s *applicationSuite) TestWatch(c *tc.C) {
	client := uniter.NewClient(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	app, err := client.Application(context.Background(), names.NewApplicationTag("mysql"))
	c.Assert(err, tc.ErrorIsNil)

	w, err := app.Watch(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	select {
	case _, ok := <-w.Changes():
		c.Assert(ok, tc.IsTrue)
	case <-time.After(testing.LongWait):
		c.Fatalf("watcher did not send change")
	}
}

func (s *applicationSuite) TestRefresh(c *tc.C) {
	client := uniter.NewClient(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	app, err := client.Application(context.Background(), names.NewApplicationTag("mysql"))
	c.Assert(err, tc.ErrorIsNil)

	s.life = life.Dying
	err = app.Refresh(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(app.Life(), tc.Equals, life.Dying)
}

func (s *applicationSuite) TestCharmURL(c *tc.C) {
	client := uniter.NewClient(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	app, err := client.Application(context.Background(), names.NewApplicationTag("mysql"))
	c.Assert(err, tc.ErrorIsNil)

	curl, force, err := app.CharmURL(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(curl, tc.Equals, "ch:mysql")
	c.Assert(force, tc.IsTrue)
}

func (s *applicationSuite) TestCharmModifiedVersion(c *tc.C) {
	client := uniter.NewClient(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	app, err := client.Application(context.Background(), names.NewApplicationTag("mysql"))
	c.Assert(err, tc.ErrorIsNil)

	ver, err := app.CharmModifiedVersion(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ver, tc.Equals, 1)
}

func (s *applicationSuite) TestSetApplicationStatus(c *tc.C) {
	client := uniter.NewClient(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	app, err := client.Application(context.Background(), names.NewApplicationTag("mysql"))
	c.Assert(err, tc.ErrorIsNil)

	err = app.SetStatus(context.Background(), "mysql/0", status.Blocked, "app blocked", map[string]interface{}{"foo": "bar"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.statusSet, tc.IsTrue)
}

func (s *applicationSuite) TestApplicationStatus(c *tc.C) {
	client := uniter.NewClient(s.apiCallerFunc(c), names.NewUnitTag("mysql/0"))
	app, err := client.Application(context.Background(), names.NewApplicationTag("mysql"))
	c.Assert(err, tc.ErrorIsNil)

	status, err := app.Status(context.Background(), "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(status, tc.DeepEquals, params.ApplicationStatusResult{
		Application: params.StatusResult{Status: "alive"},
		Units: map[string]params.StatusResult{
			"unit-mysql-0": {Status: "dying"},
		},
	})
}
