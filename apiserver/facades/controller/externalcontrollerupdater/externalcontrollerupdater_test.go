// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/externalcontrollerupdater"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/crossmodel"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&CrossControllerSuite{})

type CrossControllerSuite struct {
	coretesting.BaseSuite

	watcher             *mockStringsWatcher
	externalControllers *mockExternalControllers
	resources           *common.Resources
	auth                testing.FakeAuthorizer
	api                 *externalcontrollerupdater.ExternalControllerUpdaterAPI
}

func (s *CrossControllerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.auth = testing.FakeAuthorizer{Controller: true}
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })
	s.watcher = newMockStringsWatcher()
	s.AddCleanup(func(*gc.C) { s.watcher.Stop() })
	s.externalControllers = &mockExternalControllers{
		watcher: s.watcher,
	}
	api, err := externalcontrollerupdater.NewAPI(s.auth, s.resources, s.externalControllers)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
}

func (s *CrossControllerSuite) TestNewAPINonController(c *gc.C) {
	s.auth.Controller = false
	_, err := externalcontrollerupdater.NewAPI(s.auth, s.resources, s.externalControllers)
	c.Assert(err, gc.Equals, common.ErrPerm)
}

func (s *CrossControllerSuite) TestExternalControllerInfo(c *gc.C) {
	s.externalControllers.controllers = append(s.externalControllers.controllers, &mockExternalController{
		id: coretesting.ControllerTag.Id(),
		info: crossmodel.ControllerInfo{
			ControllerTag: coretesting.ControllerTag,
			Alias:         "foo",
			Addrs:         []string{"bar"},
			CACert:        "baz",
		},
	})

	results, err := s.api.ExternalControllerInfo(params.Entities{
		Entities: []params.Entity{
			{coretesting.ControllerTag.String()},
			{"controller-" + coretesting.ModelTag.Id()},
			{"machine-42"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ExternalControllerInfoResults{
		[]params.ExternalControllerInfoResult{{
			Result: &params.ExternalControllerInfo{
				ControllerTag: coretesting.ControllerTag.String(),
				Alias:         "foo",
				Addrs:         []string{"bar"},
				CACert:        "baz",
			},
		}, {
			Error: &params.Error{
				Code:    "not found",
				Message: `external controller "deadbeef-0bad-400d-8000-4b1d0d06f00d" not found`,
			},
		}, {
			Error: &params.Error{Message: `"machine-42" is not a valid controller tag`},
		}},
	})
}

func (s *CrossControllerSuite) TestSetExternalControllerInfo(c *gc.C) {
	s.externalControllers.controllers = append(s.externalControllers.controllers, &mockExternalController{
		id: coretesting.ControllerTag.Id(),
		info: crossmodel.ControllerInfo{
			ControllerTag: coretesting.ControllerTag,
		},
	})

	results, err := s.api.SetExternalControllerInfo(params.SetExternalControllersInfoParams{
		[]params.SetExternalControllerInfoParams{{
			params.ExternalControllerInfo{
				ControllerTag: coretesting.ControllerTag.String(),
				Alias:         "foo",
				Addrs:         []string{"bar"},
				CACert:        "baz",
			},
		}, {
			params.ExternalControllerInfo{
				ControllerTag: "controller-" + coretesting.ModelTag.Id(),
				Alias:         "qux",
				Addrs:         []string{"quux"},
				CACert:        "quuz",
			},
		}, {
			params.ExternalControllerInfo{
				ControllerTag: "machine-42",
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		[]params.ErrorResult{
			{nil},
			{nil},
			{Error: &params.Error{Message: `"machine-42" is not a valid controller tag`}},
		},
	})

	c.Assert(
		s.externalControllers.controllers,
		jc.DeepEquals,
		[]*mockExternalController{{
			id: coretesting.ControllerTag.Id(),
			info: crossmodel.ControllerInfo{
				ControllerTag: coretesting.ControllerTag,
				Alias:         "foo",
				Addrs:         []string{"bar"},
				CACert:        "baz",
			},
		}, {
			id: coretesting.ModelTag.Id(),
			info: crossmodel.ControllerInfo{
				ControllerTag: names.NewControllerTag(coretesting.ModelTag.Id()),
				Alias:         "qux",
				Addrs:         []string{"quux"},
				CACert:        "quuz",
			},
		}},
	)
}

func (s *CrossControllerSuite) TestWatchExternalControllers(c *gc.C) {
	s.watcher.changes <- []string{"a", "b"} // initial value
	results, err := s.api.WatchExternalControllers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringsWatchResults{
		[]params.StringsWatchResult{{
			StringsWatcherId: "1",
			Changes:          []string{"a", "b"},
		}},
	})
	c.Assert(s.resources.Get("1"), gc.Equals, s.watcher)
}

func (s *CrossControllerSuite) TestWatchControllerInfoError(c *gc.C) {
	s.watcher.tomb.Kill(errors.New("nope"))
	close(s.watcher.changes)

	results, err := s.api.WatchExternalControllers()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringsWatchResults{
		[]params.StringsWatchResult{{
			Error: &params.Error{Message: "nope"},
		}},
	})
	c.Assert(s.resources.Get("1"), gc.IsNil)
}
