// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"os"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/fs"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6/hooks"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator/commands"
	"github.com/juju/juju/worker/caasoperator/hook"
	"github.com/juju/juju/worker/caasoperator/runner/context"
	"github.com/juju/juju/worker/caasoperator/runner/runnertesting"
)

type ContextFactorySuite struct {
	HookContextSuite

	paths      runnertesting.MockPaths
	factory    context.ContextFactory
	membership map[int][]string
}

var _ = gc.Suite(&ContextFactorySuite{})

func (s *ContextFactorySuite) SetUpTest(c *gc.C) {
	s.HookContextSuite.SetUpTest(c)
	s.paths = runnertesting.NewMockPaths(c)
	s.membership = map[int][]string{}

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		ContextFactoryAPI: s.contextAPI,
		ApplicationTag:    names.NewApplicationTag("gitlab"),
		ModelName:         "gitlab-model",
		ModelUUID:         coretesting.ModelTag.Id(),
		GetRelationInfos:  s.getRelationInfos,
		Paths:             s.paths,
		Clock:             testing.NewClock(time.Time{}),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.factory = contextFactory
}

func (s *ContextFactorySuite) setUpCacheMethods(c *gc.C) {
	// The factory's caches are created lazily, so it doesn't have any at all to
	// begin with. Creating and discarding a context lets us call updateCache
	// without panicking. (IMO this is less invasive that making updateCache
	// responsible for creating missing caches etc.)
	_, err := s.factory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ContextFactorySuite) updateCache(relId int, unitName string, settings map[string]string) {
	context.UpdateCachedSettings(s.factory, relId, unitName, settings)
}

func (s *ContextFactorySuite) getCache(relId int, unitName string) (commands.Settings, bool) {
	return context.CachedSettings(s.factory, relId, unitName)
}

func (s *ContextFactorySuite) SetCharm(c *gc.C, name string) {
	err := os.RemoveAll(s.paths.GetCharmDir())
	c.Assert(err, jc.ErrorIsNil)
	err = fs.Copy(testcharms.Repo.CharmDirPath(name), s.paths.GetCharmDir())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ContextFactorySuite) getRelationInfos() map[int]*context.RelationInfo {
	info := map[int]*context.RelationInfo{}
	for relId, relUnit := range s.relationAPIs {
		info[relId] = &context.RelationInfo{
			RelationUnitAPI: relUnit,
			MemberNames:     s.membership[relId],
		}
	}
	return info
}

func (s *ContextFactorySuite) TestRelationHookContext(c *gc.C) {
	hi := hook.Info{
		Kind:       hooks.RelationChanged,
		RelationId: 1,
		RemoteUnit: "mysql",
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertRelationContext(c, ctx, 1, "mysql")
}

func (s *ContextFactorySuite) TestCommandContext(c *gc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
	c.Assert(err, jc.ErrorIsNil)

	s.AssertCoreContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
}

func (s *ContextFactorySuite) TestCommandContextNoRelation(c *gc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextForceNoRemoteUnit(c *gc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{
		RelationId: 0, ForceRemoteUnit: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "")
}

func (s *ContextFactorySuite) TestNewCommandContextForceRemoteUnitMissing(c *gc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{
		RelationId: 0, RemoteUnitName: "blah/123", ForceRemoteUnit: true,
	})
	c.Assert(err, gc.IsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "blah/123")
}

func (s *ContextFactorySuite) TestNewCommandContextInferRemoteUnit(c *gc.C) {
	s.membership[0] = []string{"foo/2"}
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: 0})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "foo/2")
}

func (s *ContextFactorySuite) TestNewHookContextPrunesNonMemberCaches(c *gc.C) {

	// Write cached member settings for a member and a non-member.
	s.setUpCacheMethods(c)
	s.membership[0] = []string{"rel0/0"}
	s.updateCache(0, "rel0/0", map[string]string{"keep": "me"})
	s.updateCache(0, "rel0/1", map[string]string{"drop": "me"})

	ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)

	settings0, found := s.getCache(0, "rel0/0")
	c.Assert(found, jc.IsTrue)
	c.Assert(settings0.Map(), jc.DeepEquals, map[string]string{"keep": "me"})

	settings1, found := s.getCache(0, "rel0/1")
	c.Assert(found, jc.IsFalse)
	c.Assert(settings1, gc.IsNil)

	// Check the caches are being used by the context relations.
	relCtx, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)

	// Verify that the settings really were cached by trying to look them up.
	// Nothing's really in scope, so the call would fail if they weren't.
	settings0, err = relCtx.RemoteSettings("rel0/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings0.Map(), jc.DeepEquals, map[string]string{"keep": "me"})

	// Verify that the non-member settings were purged by looking them up and
	// checking for the expected error.
	settings1, err = relCtx.RemoteSettings("rel0/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings1, gc.HasLen, 0)
}

func (s *ContextFactorySuite) TestNewHookContextRelationChangedUpdatesRelationContextAndCaches(c *gc.C) {
	// Update member settings to have actual values, so we can check that
	// the change for r/4 clears its cache but leaves r/0's alone.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", map[string]string{"foo": "bar"})
	s.updateCache(1, "r/4", map[string]string{"baz": "qux"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:       hooks.RelationChanged,
		RelationId: 1,
		RemoteUnit: "r/4",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/4")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0", "r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0.Map(), jc.DeepEquals, map[string]string{"foo": "bar"})
	c.Assert(member, jc.IsTrue)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, gc.IsNil)
	c.Assert(member, jc.IsTrue)
}
