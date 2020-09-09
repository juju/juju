// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type applicationWatcherSuite struct{}

var _ = gc.Suite(&applicationWatcherSuite{})

func (s *applicationWatcherSuite) TestEmbeddedFilter(c *gc.C) {
	app1 := &mockAppWatcherApplication{
		charm: mockAppWatcherCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentMode: charm.ModeEmbedded,
				},
			},
		},
	}
	app2 := &mockAppWatcherApplication{
		charm: mockAppWatcherCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentMode: charm.ModeWorkload,
				},
			},
		},
	}
	ch := make(chan []string, 1)
	ch <- []string{"application-two", "application-one"}
	state := &mockAppWatcherState{
		apps: map[string]*mockAppWatcherApplication{
			"application-one": app1,
			"application-two": app2,
		},
		watcher: statetesting.NewMockStringsWatcher(ch),
	}
	resources := &mockAppWatcherResources{}
	defer resources.Cleanup(c)
	f := common.NewApplicationWatcherFacade(state, resources, common.ApplicationFilterCAASEmbedded)
	watcher, err := f.WatchApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watcher, gc.NotNil)
	c.Assert(watcher.Changes, jc.SameContents, []string{"application-one"})

	state.CheckCallNames(c, "WatchApplications", "Application", "Application")
	app1.CheckCallNames(c, "Charm")
	app2.CheckCallNames(c, "Charm")
}

func (s *applicationWatcherSuite) TestLegacyFilter(c *gc.C) {
	app1 := &mockAppWatcherApplication{
		charm: mockAppWatcherCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentMode: charm.ModeEmbedded,
				},
			},
		},
	}
	app2 := &mockAppWatcherApplication{
		charm: mockAppWatcherCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentMode: charm.ModeWorkload,
				},
			},
		},
	}
	ch := make(chan []string, 1)
	ch <- []string{"application-two", "application-one"}
	state := &mockAppWatcherState{
		apps: map[string]*mockAppWatcherApplication{
			"application-one": app1,
			"application-two": app2,
		},
		watcher: statetesting.NewMockStringsWatcher(ch),
	}
	resources := &mockAppWatcherResources{}
	defer resources.Cleanup(c)
	f := common.NewApplicationWatcherFacade(state, resources, common.ApplicationFilterCAASLegacy)
	watcher, err := f.WatchApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watcher, gc.NotNil)
	c.Assert(watcher.Changes, jc.SameContents, []string{"application-two"})

	state.CheckCallNames(c, "WatchApplications", "Application", "Application")
	app1.CheckCallNames(c, "Charm")
	app2.CheckCallNames(c, "Charm")
}

func (s *applicationWatcherSuite) TestNoFilter(c *gc.C) {
	app1 := &mockAppWatcherApplication{
		charm: mockAppWatcherCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentMode: charm.ModeEmbedded,
				},
			},
		},
	}
	app2 := &mockAppWatcherApplication{
		charm: mockAppWatcherCharm{
			meta: &charm.Meta{
				Deployment: &charm.Deployment{
					DeploymentMode: charm.ModeWorkload,
				},
			},
		},
	}
	ch := make(chan []string, 1)
	ch <- []string{"application-two", "application-one"}
	state := &mockAppWatcherState{
		apps: map[string]*mockAppWatcherApplication{
			"application-one": app1,
			"application-two": app2,
		},
		watcher: statetesting.NewMockStringsWatcher(ch),
	}
	resources := &mockAppWatcherResources{}
	defer resources.Cleanup(c)
	f := common.NewApplicationWatcherFacade(state, resources, common.ApplicationFilterNone)
	watcher, err := f.WatchApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watcher, gc.NotNil)
	c.Assert(watcher.Changes, jc.SameContents, []string{"application-two", "application-one"})

	state.CheckCallNames(c, "WatchApplications")
	app1.CheckCallNames(c)
	app2.CheckCallNames(c)
}

type mockAppWatcherState struct {
	testing.Stub
	apps    map[string]*mockAppWatcherApplication
	watcher *statetesting.MockStringsWatcher
}

func (s *mockAppWatcherState) WatchApplications() state.StringsWatcher {
	s.MethodCall(s, "WatchApplications")
	return s.watcher
}

func (s *mockAppWatcherState) Application(name string) (common.AppWatcherApplication, error) {
	s.MethodCall(s, "Application", name)
	app, ok := s.apps[name]
	if !ok {
		return nil, errors.NotFoundf("application %s", name)
	}
	return app, nil
}

type mockAppWatcherApplication struct {
	testing.Stub
	force bool
	charm mockAppWatcherCharm
}

func (s *mockAppWatcherApplication) Charm() (common.AppWatcherCharm, bool, error) {
	s.MethodCall(s, "Charm")
	err := s.NextErr()
	if err != nil {
		return nil, false, err
	}
	return &s.charm, s.force, nil
}

type mockAppWatcherCharm struct {
	testing.Stub
	meta *charm.Meta
}

func (s *mockAppWatcherCharm) Meta() *charm.Meta {
	s.MethodCall(s, "Meta")
	return s.meta
}

type mockAppWatcherResources struct {
	facade.Resources
	testing.Stub
	cleanup []facade.Resource
}

func (r *mockAppWatcherResources) Register(resource facade.Resource) string {
	r.MethodCall(r, "Register", resource)
	r.cleanup = append(r.cleanup, resource)
	return "resource"
}

func (r *mockAppWatcherResources) Cleanup(c *gc.C) {
	for _, rsc := range r.cleanup {
		err := rsc.Stop()
		c.Assert(err, jc.ErrorIsNil)
	}
}
