// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"time"

	"github.com/juju/charm/v11/hooks"
	"github.com/juju/clock/testclock"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner/context"
	contextmocks "github.com/juju/juju/worker/uniter/runner/context/mocks"
	runnertesting "github.com/juju/juju/worker/uniter/runner/testing"
)

type ContextFactorySuite struct {
	BaseHookContextSuite
	paths      runnertesting.RealPaths
	factory    context.ContextFactory
	membership map[int][]string
	modelType  model.ModelType
}

var _ = gc.Suite(&ContextFactorySuite{})

func (s *ContextFactorySuite) SetUpTest(c *gc.C) {
	s.BaseHookContextSuite.SetUpTest(c)
	s.paths = runnertesting.NewRealPaths(c)
	s.membership = map[int][]string{}
	s.modelType = model.IAAS
}

func (s *ContextFactorySuite) setupContextFactory(c *gc.C, ctrl *gomock.Controller) {
	s.setupUniter(ctrl)

	s.unit.EXPECT().PrincipalName().Return("", false, nil)
	s.uniter.EXPECT().Model().Return(&model.Model{
		Name:      "test-model",
		UUID:      coretesting.ModelTag.Id(),
		ModelType: s.modelType,
	}, nil)
	s.uniter.EXPECT().LeadershipSettings().Return(&stubLeadershipSettingsAccessor{}).AnyTimes()
	s.uniter.EXPECT().APIAddresses().Return([]string{"10.6.6.6"}, nil).AnyTimes()
	s.uniter.EXPECT().SLALevel().Return("essential", nil).AnyTimes()
	s.uniter.EXPECT().CloudAPIVersion().Return("6.6.6", nil).AnyTimes()

	cfg := coretesting.ModelConfig(c)
	s.uniter.EXPECT().ModelConfig().Return(cfg, nil).AnyTimes()

	s.payloads = contextmocks.NewMockPayloadAPIClient(ctrl)
	s.payloads.EXPECT().List().Return(nil, nil).AnyTimes()

	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		Uniter:           s.uniter,
		Unit:             s.unit,
		Tracker:          &runnertesting.FakeTracker{},
		GetRelationInfos: s.getRelationInfos,
		SecretsClient:    s.secrets,
		Payloads:         s.payloads,
		Paths:            s.paths,
		Clock:            testclock.NewClock(time.Time{}),
		Logger:           loggo.GetLogger("test"),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.factory = contextFactory

	s.AddContextRelation(c, ctrl, "db0")
	s.AddContextRelation(c, ctrl, "db1")
}

func (s *ContextFactorySuite) setupCacheMethods(c *gc.C) {
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

func (s *ContextFactorySuite) updateAppCache(relId int, unitName string, settings params.Settings) {
	context.UpdateCachedAppSettings(s.factory, relId, unitName, settings)
}

func (s *ContextFactorySuite) getCache(relId int, unitName string) (params.Settings, bool) {
	return context.CachedSettings(s.factory, relId, unitName)
}

func (s *ContextFactorySuite) getAppCache(relId int, appName string) (params.Settings, bool) {
	return context.CachedAppSettings(s.factory, relId, appName)
}

func (s *ContextFactorySuite) getRelationInfos() map[int]*context.RelationInfo {
	info := map[int]*context.RelationInfo{}
	for relId, relUnit := range s.relunits {
		info[relId] = &context.RelationInfo{
			RelationUnit: relUnit,
			MemberNames:  s.membership[relId],
		}
	}
	return info
}

func (s *ContextFactorySuite) TestNewHookContextRetrievesSLALevel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.SLALevel(), gc.Equals, "essential")
}

func (s *ContextFactorySuite) TestRelationHookContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	hi := hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 1, "", "")
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	s.AssertNotSecretContext(c, ctx)
}

func (s *ContextFactorySuite) TestWorkloadHookContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	hi := hook.Info{
		Kind:         hooks.PebbleReady,
		WorkloadName: "test",
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertWorkloadContext(c, ctx, "test")
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotSecretContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewHookContextWithStorage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	s.uniter.EXPECT().StorageAttachment(names.NewStorageTag("data/0"), names.NewUnitTag("u/0")).Return(params.StorageAttachment{
		Kind:     params.StorageKindBlock,
		Location: "/dev/sdb",
	}, nil).AnyTimes()

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: "data/0",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
	c.Assert(ctx.ModelType(), gc.Equals, model.IAAS)
	s.AssertStorageContext(c, ctx, "data/0", storage.StorageAttachmentInfo{
		Kind:     storage.StorageKindBlock,
		Location: "/dev/sdb",
	})
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotSecretContext(c, ctx)
}

func (s *ContextFactorySuite) TestSecretHookContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	hi := hook.Info{
		// Kind can be any secret hook kind.
		// Whatever attributes are set below will
		// be added to the context.
		Kind:           hooks.SecretExpired,
		SecretURI:      "secret:9m4e2mr0ui3e8a215n4g",
		SecretLabel:    "label",
		SecretRevision: 666,
	}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertSecretContext(c, ctx, hi.SecretURI, hi.SecretLabel, hi.SecretRevision)
	s.AssertNotWorkloadContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewHookContextCAASModel(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.modelType = model.CAAS
	s.setupContextFactory(c, ctrl)

	ctx, err := s.factory.HookContext(hook.Info{
		Kind: hooks.ConfigChanged,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.UnitName(), gc.Equals, s.unit.Name())
	c.Assert(ctx.ModelType(), gc.Equals, model.CAAS)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestActionContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	action := apiuniter.NewAction("666", "backup", nil, false, "")
	actionData := &context.ActionData{
		Name:       action.Name(),
		Tag:        names.NewActionTag(action.ID()),
		Params:     action.Params(),
		ResultsMap: map[string]interface{}{},
	}

	ctx, err := s.factory.ActionContext(actionData)
	c.Assert(err, jc.ErrorIsNil)

	s.AssertCoreContext(c, ctx)
	s.AssertActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestCommandContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
	c.Assert(err, jc.ErrorIsNil)

	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestCommandContextNoRelation(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: -1})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextForceNoRemoteUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	ctx, err := s.factory.CommandContext(context.CommandInfo{
		RelationId: 0, ForceRemoteUnit: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "", "")
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextForceRemoteUnitMissing(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	ctx, err := s.factory.CommandContext(context.CommandInfo{
		// TODO(jam): 2019-10-23 Add RemoteApplicationName
		RelationId: 0, RemoteUnitName: "blah/123", ForceRemoteUnit: true,
	})
	c.Assert(err, gc.IsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "blah/123", "")
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewCommandContextInferRemoteUnit(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	// TODO(jam): 2019-10-23 Add RemoteApplicationName
	s.membership[0] = []string{"foo/2"}
	ctx, err := s.factory.CommandContext(context.CommandInfo{RelationId: 0})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "foo/2", "")
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
}

func (s *ContextFactorySuite) TestNewHookContextPrunesNonMemberCaches(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	// Write cached member settings for a member and a non-member.
	s.setupCacheMethods(c)
	s.membership[0] = []string{"rel0/0"}
	s.updateCache(0, "rel0/0", params.Settings{"keep": "me"})
	s.updateCache(0, "rel0/1", params.Settings{"drop": "me"})

	s.relunits[0].EXPECT().ReadSettings("rel0/0").Return(nil, nil).AnyTimes()
	s.relunits[0].EXPECT().ReadSettings("rel0/1").Return(nil, nil).AnyTimes()

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
}

func (s *ContextFactorySuite) TestNewHookContextRelationJoinedUpdatesRelationContextAndCaches(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	// Write some cached settings for r/0, so we can verify the cache gets cleared.
	s.setupCacheMethods(c)
	s.membership[1] = []string{"r/0"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:              hooks.RelationJoined,
		RelationId:        1,
		RemoteUnit:        "r/0",
		RemoteApplication: "r",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/0", "r")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, gc.IsNil)
	c.Assert(member, jc.IsTrue)
}

func (s *ContextFactorySuite) TestNewHookContextRelationChangedUpdatesRelationContextAndCaches(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	// Update member settings to have actual values, so we can check that
	// the change for r/4 clears its cache but leaves r/0's alone.
	s.setupCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})
	s.updateAppCache(1, "r", params.Settings{"frob": "nizzle"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:              hooks.RelationChanged,
		RelationId:        1,
		RemoteUnit:        "r/4",
		RemoteApplication: "r",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/4", "r")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0", "r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, jc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(member, jc.IsTrue)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, gc.IsNil)
	c.Assert(member, jc.IsTrue)
	wrongCache, member := s.getCache(1, "r")
	c.Assert(wrongCache, gc.IsNil)
	c.Assert(member, jc.IsFalse)
	cachedApp, found := s.getAppCache(1, "r")
	// TODO(jam): 2019-10-23 This is currently wrong. We are currently pruning
	//  all application settings on every hook invocation. We should only
	//  invalidate it when we run a relation-changed hook for the app
	c.ExpectFailure("application settings should be properly cached")
	c.Assert(cachedApp, jc.DeepEquals, params.Settings{"frob": "bar"})
	c.Assert(found, jc.IsTrue)
}

func (s *ContextFactorySuite) TestNewHookContextRelationChangedUpdatesRelationContextAndCachesApplication(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	// Set values for r/0 and r make sure we don't see r/0 change but we *do* see r wiped.
	s.setupCacheMethods(c)
	s.membership[1] = []string{"r/0"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateAppCache(1, "r", params.Settings{"baz": "quux"})
	cachedApp, found := s.getAppCache(1, "r")
	c.Assert(cachedApp, jc.DeepEquals, params.Settings{"baz": "quux"})
	c.Assert(found, jc.IsTrue)

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:              hooks.RelationChanged,
		RelationId:        1,
		RemoteApplication: "r",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "", "r")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, jc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(member, jc.IsTrue)
	// It should not be found in the normal cache
	wrongCache, member := s.getCache(1, "r")
	c.Assert(wrongCache, gc.IsNil)
	c.Assert(member, jc.IsFalse)
	cachedApp, found = s.getAppCache(1, "r")
	c.Assert(cachedApp, gc.IsNil)
	c.Assert(found, jc.IsFalse)
}

func (s *ContextFactorySuite) TestNewHookContextRelationDepartedUpdatesRelationContextAndCaches(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	// Update member settings to have actual values, so we can check that
	// the depart for r/0 leaves r/4's cache alone (while discarding r/0's).
	s.setupCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:          hooks.RelationDeparted,
		RelationId:    1,
		RemoteUnit:    "r/0",
		DepartingUnit: "r/0",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	s.AssertNotWorkloadContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/0", "")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, gc.IsNil)
	c.Assert(member, jc.IsFalse)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, jc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(member, jc.IsTrue)
}

func (s *ContextFactorySuite) TestNewHookContextRelationBrokenRetainsCaches(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	s.setupContextFactory(c, ctrl)

	// Note that this is bizarre and unrealistic, because we would never usually
	// run relation-broken on a non-empty relation. But verfying that the settings
	// stick around allows us to verify that there's no special handling for that
	// hook -- as there should not be, because the relation caches will be discarded
	// for the *next* hook, which will be constructed with the current set of known
	// relations and ignore everything else.
	s.setupCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})

	ctx, err := s.factory.HookContext(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	})
	c.Assert(err, jc.ErrorIsNil)
	rel := s.AssertRelationContext(c, ctx, 1, "", "")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0", "r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, jc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(member, jc.IsTrue)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, jc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(member, jc.IsTrue)
}
