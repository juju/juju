// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/fs"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable/hooks"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner/context"
	runnertesting "github.com/juju/juju/worker/uniter/runner/testing"
)

type ContextFactorySuite struct {
	HookContextSuite
	paths      runnertesting.RealPaths
	factory    context.ContextFactory
	membership map[int][]string
}

var _ = gc.Suite(&ContextFactorySuite{})

func (s *ContextFactorySuite) SetUpTest(c *gc.C) {
	s.HookContextSuite.SetUpTest(c)
	s.paths = runnertesting.NewRealPaths(c)
	s.membership = map[int][]string{}

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		State:            s.uniter,
		UnitTag:          s.unit.Tag().(names.UnitTag),
		Tracker:          runnertesting.FakeTracker{},
		GetRelationInfos: s.getRelationInfos,
		Storage:          s.storage,
		Paths:            s.paths,
		Clock:            testing.NewClock(time.Time{}),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.factory = contextFactory
}

func (s *ContextFactorySuite) setUpCacheMethods(c *gc.C) {
	// The factory's caches are created lazily, so it doesn't have any at all to
	// begin with. Creating and discarding a context lets us call updateCache
	// without panicking. (IMO this is less invasive that making updateCache
	// responsible for creating missing caches etc.)
	_, err := s.factory.HookContext(hook.Info{Kind: hooks.Install})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ContextFactorySuite) updateCache(relId int, unitName string, settings params.Settings) {
	context.UpdateCachedSettings(s.factory, relId, unitName, settings)
}

func (s *ContextFactorySuite) getCache(relId int, unitName string) (params.Settings, bool) {
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
	for relId, relUnit := range s.apiRelunits {
		info[relId] = &context.RelationInfo{
			RelationUnit: relUnit,
			MemberNames:  s.membership[relId],
		}
	}
	return info
}

func (s *ContextFactorySuite) testLeadershipContextWiring(c *gc.C, createContext func() *context.HookContext) {
	var stub testing.Stub
	stub.SetErrors(errors.New("bam"))
	restore := context.PatchNewLeadershipContext(
		func(accessor context.LeadershipSettingsAccessor, tracker leadership.Tracker) context.LeadershipContext {
			stub.AddCall("NewLeadershipContext", accessor, tracker)
			return &StubLeadershipContext{Stub: &stub}
		},
	)
	defer restore()

	ctx := createContext()
	isLeader, err := ctx.IsLeader()
	c.Check(err, gc.ErrorMatches, "bam")
	c.Check(isLeader, jc.IsFalse)

	stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "NewLeadershipContext",
		Args:     []interface{}{s.uniter.LeadershipSettings, runnertesting.FakeTracker{}},
	}, {
		FuncName: "IsLeader",
	}})

}

func (s *ContextFactorySuite) TestNewHookContextRetrievesSLALevel(c *gc.C) {
	err := s.State.SetSLA("essential", []byte("creds"))
	c.Assert(err, jc.ErrorIsNil)

	ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.SLALevel(), gc.Equals, "essential")
}

func (s *ContextFactorySuite) TestNewHookContextLeadershipContext(c *gc.C) {
	s.testLeadershipContextWiring(c, func() *context.HookContext {
		ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
		c.Assert(err, jc.ErrorIsNil)
		return ctx
	})
}

func (s *ContextFactorySuite) TestNewCommandContextLeadershipContext(c *gc.C) {
	s.testLeadershipContextWiring(c, func() *context.HookContext {
		ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
		c.Assert(err, jc.ErrorIsNil)
		return ctx
	})
}

func (s *ContextFactorySuite) TestNewActionContextLeadershipContext(c *gc.C) {
	s.testLeadershipContextWiring(c, func() *context.HookContext {
		s.SetCharm(c, "dummy")
		action, err := s.State.EnqueueAction(s.unit.Tag(), "snapshot", nil)
		c.Assert(err, jc.ErrorIsNil)

		actionData := &context.ActionData{
			Name:       action.Name(),
			Tag:        names.NewActionTag(action.Id()),
			Params:     action.Parameters(),
			ResultsMap: map[string]interface{}{},
		}

		ctx, err := s.factory.ActionContext(actionData)
		c.Assert(err, jc.ErrorIsNil)
		return ctx
	})
}

func (s *ContextFactorySuite) TestRelationHookContext(c *gc.C) {
	hi := hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 1, "")
	s.AssertNotStorageContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewHookContextWithStorage(c *gc.C) {
	// We need to set up a unit that has storage metadata defined.
	ch := s.AddTestingCharm(c, "storage-block")
	sCons := map[string]state.StorageConstraints{
		"data": {Pool: "", Size: 1024, Count: 1},
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block", ch, sCons)
	s.machine = nil // allocate a new machine
	unit := s.AddUnit(c, service)

	storageAttachments, err := s.State.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageAttachments, gc.HasLen, 1)
	storageTag := storageAttachments[0].StorageInstance()

	volume, err := s.State.StorageInstanceVolume(storageTag)
	c.Assert(err, jc.ErrorIsNil)
	volumeTag := volume.VolumeTag()
	machineTag := s.machine.MachineTag()

	err = s.State.SetVolumeInfo(
		volumeTag, state.VolumeInfo{
			VolumeId: "vol-123",
			Size:     456,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetVolumeAttachmentInfo(
		machineTag, volumeTag, state.VolumeAttachmentInfo{
			DeviceName: "sdb",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	password, err := utils.RandomPassword()
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	st := s.OpenAPIAs(c, unit.Tag(), password)
	uniter, err := st.Uniter()
	c.Assert(err, jc.ErrorIsNil)

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		State:            uniter,
		UnitTag:          unit.Tag().(names.UnitTag),
		Tracker:          runnertesting.FakeTracker{},
		GetRelationInfos: s.getRelationInfos,
		Storage:          s.storage,
		Paths:            s.paths,
		Clock:            testing.NewClock(time.Time{}),
	})
	c.Assert(err, jc.ErrorIsNil)
	ctx, err := contextFactory.HookContext(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: "data/0",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.UnitName(), gc.Equals, "storage-block/0")
	s.AssertStorageContext(c, ctx, "data/0", storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/sdb",
	})
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
}

func (s *ContextFactorySuite) TestActionContext(c *gc.C) {
	s.SetCharm(c, "dummy")
	action, err := s.State.EnqueueAction(s.unit.Tag(), "snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)

	actionData := &context.ActionData{
		Name:       action.Name(),
		Tag:        names.NewActionTag(action.Id()),
		Params:     action.Parameters(),
		ResultsMap: map[string]interface{}{},
	}

	ctx, err := s.factory.ActionContext(actionData)
	c.Assert(err, jc.ErrorIsNil)

	s.AssertCoreContext(c, ctx)
	s.AssertActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
}

func (s *ContextFactorySuite) TestCommandContext(c *gc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
	c.Assert(err, jc.ErrorIsNil)

	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
}

func (s *ContextFactorySuite) TestCommandContextNoRelation(c *gc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextForceNoRemoteUnit(c *gc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{
		RelationId: 0, ForceRemoteUnit: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "")
	s.AssertNotStorageContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextForceRemoteUnitMissing(c *gc.C) {
	ctx, err := s.factory.CommandContext(context.CommandInfo{
		RelationId: 0, RemoteUnitName: "blah/123", ForceRemoteUnit: true,
	})
	c.Assert(err, gc.IsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "blah/123")
	s.AssertNotStorageContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextInferRemoteUnit(c *gc.C) {
	s.membership[0] = []string{"foo/2"}
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: 0})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "foo/2")
	s.AssertNotStorageContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewHookContextPrunesNonMemberCaches(c *gc.C) {

	// Write cached member settings for a member and a non-member.
	s.setUpCacheMethods(c)
	s.membership[0] = []string{"rel0/0"}
	s.updateCache(0, "rel0/0", params.Settings{"keep": "me"})
	s.updateCache(0, "rel0/1", params.Settings{"drop": "me"})

	ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.Install})
	c.Assert(err, jc.ErrorIsNil)

	settings0, found := s.getCache(0, "rel0/0")
	c.Assert(found, jc.IsTrue)
	c.Assert(settings0, jc.DeepEquals, params.Settings{"keep": "me"})

	settings1, found := s.getCache(0, "rel0/1")
	c.Assert(found, jc.IsFalse)
	c.Assert(settings1, gc.IsNil)

	// Check the caches are being used by the context relations.
	relCtx, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)

	// Verify that the settings really were cached by trying to look them up.
	// Nothing's really in scope, so the call would fail if they weren't.
	settings0, err = relCtx.ReadSettings("rel0/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings0, jc.DeepEquals, params.Settings{"keep": "me"})

	// Verify that the non-member settings were purged by looking them up and
	// checking for the expected error.
	settings1, err = relCtx.ReadSettings("rel0/1")
	c.Assert(settings1, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ContextFactorySuite) TestNewHookContextRelationJoinedUpdatesRelationContextAndCaches(c *gc.C) {
	// Write some cached settings for r/0, so we can verify the cache gets cleared.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:       hooks.RelationJoined,
		RelationId: 1,
		RemoteUnit: "r/0",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/0")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, gc.IsNil)
	c.Assert(member, jc.IsTrue)
}

func (s *ContextFactorySuite) TestNewHookContextRelationChangedUpdatesRelationContextAndCaches(c *gc.C) {
	// Update member settings to have actual values, so we can check that
	// the change for r/4 clears its cache but leaves r/0's alone.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:       hooks.RelationChanged,
		RelationId: 1,
		RemoteUnit: "r/4",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/4")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0", "r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, jc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(member, jc.IsTrue)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, gc.IsNil)
	c.Assert(member, jc.IsTrue)
}

func (s *ContextFactorySuite) TestNewHookContextRelationDepartedUpdatesRelationContextAndCaches(c *gc.C) {
	// Update member settings to have actual values, so we can check that
	// the depart for r/0 leaves r/4's cache alone (while discarding r/0's).
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:       hooks.RelationDeparted,
		RelationId: 1,
		RemoteUnit: "r/0",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/0")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, gc.IsNil)
	c.Assert(member, jc.IsFalse)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, jc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(member, jc.IsTrue)
}

func (s *ContextFactorySuite) TestNewHookContextRelationBrokenRetainsCaches(c *gc.C) {
	// Note that this is bizarre and unrealistic, because we would never usually
	// run relation-broken on a non-empty relation. But verfying that the settings
	// stick around allows us to verify that there's no special handling for that
	// hook -- as there should not be, because the relation caches will be discarded
	// for the *next* hook, which will be constructed with the current set of known
	// relations and ignore everything else.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	})
	c.Assert(err, jc.ErrorIsNil)
	rel := s.AssertRelationContext(c, ctx, 1, "")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0", "r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, jc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(member, jc.IsTrue)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, jc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(member, jc.IsTrue)
}

type StubLeadershipContext struct {
	context.LeadershipContext
	*testing.Stub
}

func (stub *StubLeadershipContext) IsLeader() (bool, error) {
	stub.MethodCall(stub, "IsLeader")
	return false, stub.NextErr()
}
