// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/juju/status"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker/caasoperator/commands"
	"github.com/juju/juju/worker/caasoperator/runner/context"
)

type InterfaceSuite struct {
	HookContextSuite
	stub testing.Stub
}

var _ = gc.Suite(&InterfaceSuite{})

func (s *InterfaceSuite) TestApplicationName(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	c.Assert(ctx.ApplicationName(), gc.Equals, "gitlab")
}

func (s *InterfaceSuite) TestHookRelation(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	r, err := ctx.HookRelation()
	c.Assert(err, gc.ErrorMatches, ".*")
	c.Assert(r, gc.IsNil)
}

func (s *InterfaceSuite) TestRemoteUnitName(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	name, err := ctx.RemoteUnitName()
	c.Assert(err, gc.ErrorMatches, ".*")
	c.Assert(name, gc.Equals, "")
}

func (s *InterfaceSuite) TestRelationIds(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	relIds, err := ctx.RelationIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relIds, gc.HasLen, 2)
	r, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:0")
	r, err = ctx.Relation(123)
	c.Assert(err, gc.ErrorMatches, ".*")
	c.Assert(r, gc.IsNil)
}

func (s *InterfaceSuite) TestRelationContext(c *gc.C) {
	ctx := s.GetContext(c, 1, "")
	r, err := ctx.HookRelation()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:1")
}

func (s *InterfaceSuite) TestRelationContextWithRemoteUnitName(c *gc.C) {
	ctx := s.GetContext(c, 1, "u/123")
	name, err := ctx.RemoteUnitName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "u/123")
}

func (s *InterfaceSuite) TestNetworkInfo(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	netInfo, err := ctx.NetworkInfo([]string{"unknown"}, -1)
	c.Check(err, jc.ErrorIsNil)
	c.Check(netInfo, gc.DeepEquals, map[string]params.NetworkInfoResult{
		"db": {
			IngressAddresses: []string{"10.0.0.1"},
		},
	},
	)
}

func (s *InterfaceSuite) TestApplicationStatus(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	defer context.PatchCachedStatus(ctx.(commands.Context), "maintenance", "working", map[string]interface{}{"hello": "world"})()
	appStatus, err := ctx.ApplicationStatus()
	c.Check(err, jc.ErrorIsNil)
	c.Check(appStatus.Status, gc.Equals, "maintenance")
	c.Check(appStatus.Info, gc.Equals, "working")
	c.Check(appStatus.Data, gc.DeepEquals, map[string]interface{}{"hello": "world"})
}

func (s *InterfaceSuite) TestStatusCaching(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	appStatus, err := ctx.ApplicationStatus()
	c.Check(err, jc.ErrorIsNil)
	c.Check(appStatus.Status, gc.Equals, "maintenance")
	c.Check(appStatus.Info, gc.Equals, "initialising")
	c.Check(appStatus.Data, gc.DeepEquals, map[string]interface{}{})

	// Change remote state.
	err = s.contextAPI.SetApplicationStatus(status.Blocked, "broken", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Local view is unchanged.
	appStatus, err = ctx.ApplicationStatus()
	c.Check(err, jc.ErrorIsNil)
	c.Check(appStatus.Status, gc.Equals, "maintenance")
	c.Check(appStatus.Info, gc.Equals, "initialising")
	c.Check(appStatus.Data, gc.DeepEquals, map[string]interface{}{})
}

func (s *InterfaceSuite) TestConfigCaching(c *gc.C) {
	s.contextAPI.UpdateCharmConfig(charm.Settings{"blog-title": "My Title"})
	ctx := s.GetContext(c, -1, "")
	settings, err := ctx.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})

	// Change remote config.
	s.contextAPI.UpdateCharmConfig(charm.Settings{"blog-title": "Something Else"})

	// Local view is not changed.
	settings, err = ctx.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})
}

func (s *InterfaceSuite) TestSetContainerSpec(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	err := ctx.SetContainerSpec("gitlab/0", "spec")
	c.Check(err, jc.ErrorIsNil)
	c.Assert(s.contextAPI.Spec, gc.Equals, "spec")
	c.Assert(s.contextAPI.SpecEntityName, gc.Equals, "gitlab/0")
}

func (s *InterfaceSuite) TestSetContainerSpecApplication(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	err := ctx.SetContainerSpec("", "spec")
	c.Check(err, jc.ErrorIsNil)
	c.Assert(s.contextAPI.Spec, gc.Equals, "spec")
	c.Assert(s.contextAPI.SpecEntityName, gc.Equals, "gitlab")
}
