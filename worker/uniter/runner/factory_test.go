// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"os"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/fs"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner"
)

type FactorySuite struct {
	HookContextSuite
	paths      RealPaths
	factory    runner.Factory
	membership map[int][]string
}

var _ = gc.Suite(&FactorySuite{})

func (s *FactorySuite) SetUpTest(c *gc.C) {
	s.PatchEnvironment(osenv.JujuFeatureFlagEnvKey, "storage")
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	s.HookContextSuite.SetUpTest(c)
	s.paths = NewRealPaths(c)
	s.membership = map[int][]string{}
	factory, err := runner.NewFactory(
		s.uniter,
		s.unit.Tag().(names.UnitTag),
		s.getRelationInfos,
		s.paths,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.factory = factory
}

func (s *FactorySuite) SetCharm(c *gc.C, name string) {
	err := os.RemoveAll(s.paths.charm)
	c.Assert(err, jc.ErrorIsNil)
	err = fs.Copy(testcharms.Repo.CharmDirPath(name), s.paths.charm)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FactorySuite) getRelationInfos() map[int]*runner.RelationInfo {
	info := map[int]*runner.RelationInfo{}
	for relId, relUnit := range s.apiRelunits {
		info[relId] = &runner.RelationInfo{
			RelationUnit: relUnit,
			MemberNames:  s.membership[relId],
		}
	}
	return info
}

func (s *FactorySuite) setUpCacheMethods(c *gc.C) {
	// The factory's caches are created lazily, so it doesn't have any at all to
	// begin with. Creating and discarding a context lets us call updateCache
	// without panicking. (IMO this is less invasive that making updateCache
	// responsible for creating missing caches etc.)
	_, err := s.factory.NewHookRunner(hook.Info{Kind: hooks.Install})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FactorySuite) updateCache(relId int, unitName string, settings params.Settings) {
	runner.UpdateCachedSettings(s.factory, relId, unitName, settings)
}

func (s *FactorySuite) getCache(relId int, unitName string) (params.Settings, bool) {
	return runner.CachedSettings(s.factory, relId, unitName)
}

func (s *FactorySuite) AssertPaths(c *gc.C, rnr runner.Runner) {
	c.Assert(runner.RunnerPaths(rnr), gc.DeepEquals, s.paths)
}

func (s *FactorySuite) AssertCoreContext(c *gc.C, ctx runner.Context) {
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
	c.Assert(ctx.OwnerTag(), gc.Equals, s.service.GetOwnerTag())
	c.Assert(runner.ContextMachineTag(ctx), jc.DeepEquals, names.NewMachineTag("0"))

	expect, expectOK := s.unit.PrivateAddress()
	actual, actualOK := ctx.PrivateAddress()
	c.Assert(actual, gc.Equals, expect)
	c.Assert(actualOK, gc.Equals, expectOK)

	expect, expectOK = s.unit.PublicAddress()
	actual, actualOK = ctx.PublicAddress()
	c.Assert(actual, gc.Equals, expect)
	c.Assert(actualOK, gc.Equals, expectOK)

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	name, uuid := runner.ContextEnvInfo(ctx)
	c.Assert(name, gc.Equals, env.Name())
	c.Assert(uuid, gc.Equals, env.UUID())

	c.Assert(ctx.RelationIds(), gc.HasLen, 2)

	r, found := ctx.Relation(0)
	c.Assert(found, jc.IsTrue)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:0")

	r, found = ctx.Relation(1)
	c.Assert(found, jc.IsTrue)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:1")
}

func (s *FactorySuite) AssertNotActionContext(c *gc.C, ctx runner.Context) {
	actionData, err := ctx.ActionData()
	c.Assert(actionData, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "not running an action")
}

func (s *FactorySuite) AssertNotStorageContext(c *gc.C, ctx runner.Context) {
	storageAttachment, ok := ctx.HookStorageAttachment()
	c.Assert(storageAttachment, gc.IsNil)
	c.Assert(ok, jc.IsFalse)
}

func (s *FactorySuite) AssertStorageContext(c *gc.C, ctx runner.Context, attachment params.StorageAttachment) {
	fromCache, ok := ctx.HookStorageAttachment()
	c.Assert(ok, jc.IsTrue)
	c.Assert(attachment, jc.DeepEquals, *fromCache)
}

func (s *FactorySuite) AssertRelationContext(c *gc.C, ctx runner.Context, relId int, remoteUnit string) *runner.ContextRelation {
	actualRemoteUnit, _ := ctx.RemoteUnitName()
	c.Assert(actualRemoteUnit, gc.Equals, remoteUnit)
	rel, found := ctx.HookRelation()
	c.Assert(found, jc.IsTrue)
	c.Assert(rel.Id(), gc.Equals, relId)
	return rel.(*runner.ContextRelation)
}

func (s *FactorySuite) AssertNotRelationContext(c *gc.C, ctx runner.Context) {
	rel, found := ctx.HookRelation()
	c.Assert(rel, gc.IsNil)
	c.Assert(found, jc.IsFalse)
}

func (s *FactorySuite) TestNewCommandRunnerNoRelation(c *gc.C) {
	rnr, err := s.factory.NewCommandRunner(runner.CommandInfo{RelationId: -1})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
}

func (s *FactorySuite) TestNewCommandRunnerRelationIdDoesNotExist(c *gc.C) {
	for _, value := range []bool{true, false} {
		_, err := s.factory.NewCommandRunner(runner.CommandInfo{
			RelationId: 12, ForceRemoteUnit: value,
		})
		c.Check(err, gc.ErrorMatches, `unknown relation id: 12`)
	}
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitInvalid(c *gc.C) {
	for _, value := range []bool{true, false} {
		_, err := s.factory.NewCommandRunner(runner.CommandInfo{
			RelationId: 0, RemoteUnitName: "blah", ForceRemoteUnit: value,
		})
		c.Check(err, gc.ErrorMatches, `invalid remote unit: blah`)
	}
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitInappropriate(c *gc.C) {
	for _, value := range []bool{true, false} {
		_, err := s.factory.NewCommandRunner(runner.CommandInfo{
			RelationId: -1, RemoteUnitName: "blah/123", ForceRemoteUnit: value,
		})
		c.Check(err, gc.ErrorMatches, `remote unit provided without a relation: blah/123`)
	}
}

func (s *FactorySuite) TestNewCommandRunnerEmptyRelation(c *gc.C) {
	_, err := s.factory.NewCommandRunner(runner.CommandInfo{RelationId: 1})
	c.Check(err, gc.ErrorMatches, `cannot infer remote unit in empty relation 1`)
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitAmbiguous(c *gc.C) {
	s.membership[1] = []string{"foo/0", "foo/1"}
	_, err := s.factory.NewCommandRunner(runner.CommandInfo{RelationId: 1})
	c.Check(err, gc.ErrorMatches, `ambiguous remote unit; possibilities are \[foo/0 foo/1\]`)
}

func (s *FactorySuite) TestNewCommandRunnerRemoteUnitMissing(c *gc.C) {
	s.membership[0] = []string{"foo/0", "foo/1"}
	_, err := s.factory.NewCommandRunner(runner.CommandInfo{
		RelationId: 0, RemoteUnitName: "blah/123",
	})
	c.Check(err, gc.ErrorMatches, `unknown remote unit blah/123; possibilities are \[foo/0 foo/1\]`)
}

func (s *FactorySuite) TestNewCommandRunnerForceNoRemoteUnit(c *gc.C) {
	rnr, err := s.factory.NewCommandRunner(runner.CommandInfo{
		RelationId: 0, ForceRemoteUnit: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "")
	s.AssertNotStorageContext(c, ctx)
}

func (s *FactorySuite) TestNewCommandRunnerForceRemoteUnitMissing(c *gc.C) {
	rnr, err := s.factory.NewCommandRunner(runner.CommandInfo{
		RelationId: 0, RemoteUnitName: "blah/123", ForceRemoteUnit: true,
	})
	c.Assert(err, gc.IsNil)
	ctx := rnr.Context()
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "blah/123")
	s.AssertNotStorageContext(c, ctx)
}

func (s *FactorySuite) TestNewCommandRunnerInferRemoteUnit(c *gc.C) {
	s.membership[0] = []string{"foo/2"}
	rnr, err := s.factory.NewCommandRunner(runner.CommandInfo{RelationId: 0})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0, "foo/2")
	s.AssertNotStorageContext(c, ctx)
}

func (s *FactorySuite) TestNewHookRunner(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
}

func (s *FactorySuite) TestNewHookRunnerWithBadHook(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{})
	c.Assert(rnr, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `unknown hook kind ""`)
}

func (s *FactorySuite) TestNewHookRunnerWithStorage(c *gc.C) {
	// We need to set up a unit that has storage metadata defined.
	ch := s.AddTestingCharm(c, "storage-block")
	sCons := map[string]state.StorageConstraints{
		"data": {Pool: "", Size: 1024, Count: 1},
	}
	service := s.AddTestingServiceWithStorage(c, "storage-block", ch, sCons)
	s.machine = nil // allocate a new machine
	unit := s.AddUnit(c, service)

	storageAttachments, err := s.State.StorageAttachments(unit.UnitTag())
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

	factory, err := runner.NewFactory(
		uniter,
		unit.Tag().(names.UnitTag),
		s.getRelationInfos,
		s.paths,
	)
	c.Assert(err, jc.ErrorIsNil)

	s.PatchEnvironment(osenv.JujuFeatureFlagEnvKey, "storage")
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	rnr, err := factory.NewHookRunner(hook.Info{
		Kind:      hooks.StorageAttached,
		StorageId: "data/0",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	c.Assert(ctx.UnitName(), gc.Equals, "storage-block/0")
	s.AssertStorageContext(c, ctx, params.StorageAttachment{
		StorageTag: "storage-data-0",
		OwnerTag:   unit.Tag().String(),
		UnitTag:    unit.Tag().String(),
		Kind:       params.StorageKindBlock,
		Location:   "/dev/sdb",
		Life:       "alive",
	})
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
}

func (s *FactorySuite) TestNewHookRunnerWithRelation(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 1, "")
	s.AssertNotStorageContext(c, ctx)
}

func (s *FactorySuite) TestNewHookRunnerPrunesNonMemberCaches(c *gc.C) {

	// Write cached member settings for a member and a non-member.
	s.setUpCacheMethods(c)
	s.membership[0] = []string{"rel0/0"}
	s.updateCache(0, "rel0/0", params.Settings{"keep": "me"})
	s.updateCache(0, "rel0/1", params.Settings{"drop": "me"})

	rnr, err := s.factory.NewHookRunner(hook.Info{Kind: hooks.Install})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()

	settings0, found := s.getCache(0, "rel0/0")
	c.Assert(found, jc.IsTrue)
	c.Assert(settings0, jc.DeepEquals, params.Settings{"keep": "me"})

	settings1, found := s.getCache(0, "rel0/1")
	c.Assert(found, jc.IsFalse)
	c.Assert(settings1, gc.IsNil)

	// Check the caches are being used by the context relations.
	relCtx, found := ctx.Relation(0)
	c.Assert(found, jc.IsTrue)

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

func (s *FactorySuite) TestNewHookRunnerRelationJoinedUpdatesRelationContextAndCaches(c *gc.C) {
	// Write some cached settings for r/0, so we can verify the cache gets cleared.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})

	rnr, err := s.factory.NewHookRunner(hook.Info{
		Kind:       hooks.RelationJoined,
		RelationId: 1,
		RemoteUnit: "r/0",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
	rel := s.AssertRelationContext(c, ctx, 1, "r/0")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, gc.IsNil)
	c.Assert(member, jc.IsTrue)
}

func (s *FactorySuite) TestNewHookRunnerRelationChangedUpdatesRelationContextAndCaches(c *gc.C) {
	// Update member settings to have actual values, so we can check that
	// the change for r/4 clears its cache but leaves r/0's alone.
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})

	rnr, err := s.factory.NewHookRunner(hook.Info{
		Kind:       hooks.RelationChanged,
		RelationId: 1,
		RemoteUnit: "r/4",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
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

func (s *FactorySuite) TestNewHookRunnerRelationDepartedUpdatesRelationContextAndCaches(c *gc.C) {
	// Update member settings to have actual values, so we can check that
	// the depart for r/0 leaves r/4's cache alone (while discarding r/0's).
	s.setUpCacheMethods(c)
	s.membership[1] = []string{"r/0", "r/4"}
	s.updateCache(1, "r/0", params.Settings{"foo": "bar"})
	s.updateCache(1, "r/4", params.Settings{"baz": "qux"})

	rnr, err := s.factory.NewHookRunner(hook.Info{
		Kind:       hooks.RelationDeparted,
		RelationId: 1,
		RemoteUnit: "r/0",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
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

func (s *FactorySuite) TestNewHookRunnerRelationBrokenRetainsCaches(c *gc.C) {
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

	rnr, err := s.factory.NewHookRunner(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	rel := s.AssertRelationContext(c, ctx, 1, "")
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0", "r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, jc.DeepEquals, params.Settings{"foo": "bar"})
	c.Assert(member, jc.IsTrue)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, jc.DeepEquals, params.Settings{"baz": "qux"})
	c.Assert(member, jc.IsTrue)
}

func (s *FactorySuite) TestNewHookRunnerWithBadRelation(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 12345,
	})
	c.Assert(rnr, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `unknown relation id: 12345`)
}

func (s *FactorySuite) TestNewHookRunnerMetricsDisabledHook(c *gc.C) {
	s.SetCharm(c, "metered")
	rnr, err := s.factory.NewHookRunner(hook.Info{Kind: hooks.Install})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	err = ctx.AddMetric("key", "value", time.Now())
	c.Assert(err, gc.ErrorMatches, "metrics disabled")
}

func (s *FactorySuite) TestNewHookRunnerMetricsDisabledUndeclared(c *gc.C) {
	s.SetCharm(c, "mysql")
	rnr, err := s.factory.NewHookRunner(hook.Info{Kind: hooks.CollectMetrics})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	err = ctx.AddMetric("key", "value", time.Now())
	c.Assert(err, gc.ErrorMatches, "metrics disabled")
}

func (s *FactorySuite) TestNewHookRunnerMetricsDeclarationError(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{Kind: hooks.CollectMetrics})
	c.Assert(errors.Cause(err), jc.Satisfies, os.IsNotExist)
	c.Assert(rnr, gc.IsNil)
}

func (s *FactorySuite) TestNewHookRunnerMetricsEnabled(c *gc.C) {
	s.SetCharm(c, "metered")

	rnr, err := s.factory.NewHookRunner(hook.Info{Kind: hooks.CollectMetrics})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	err = ctx.AddMetric("pings", "0.5", time.Now())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FactorySuite) TestNewActionRunnerGood(c *gc.C) {
	s.SetCharm(c, "dummy")
	action, err := s.State.EnqueueAction(s.unit.Tag(), "snapshot", map[string]interface{}{
		"outfile": "/some/file.bz2",
	})
	c.Assert(err, jc.ErrorIsNil)
	rnr, err := s.factory.NewActionRunner(action.Id())
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	data, err := ctx.ActionData()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, &runner.ActionData{
		ActionName: "snapshot",
		ActionTag:  action.ActionTag(),
		ActionParams: map[string]interface{}{
			"outfile": "/some/file.bz2",
		},
		ResultsMap: map[string]interface{}{},
	})
	vars := ctx.HookVars(s.paths)
	c.Assert(len(vars) > 0, jc.IsTrue, gc.Commentf("expected HookVars but found none"))
	combined := strings.Join(vars, "|")
	c.Assert(combined, gc.Matches, `(^|.*\|)JUJU_ACTION_NAME=snapshot(\|.*|$)`)
	c.Assert(combined, gc.Matches, `(^|.*\|)JUJU_ACTION_UUID=`+action.Id()+`(\|.*|$)`)
	c.Assert(combined, gc.Matches, `(^|.*\|)JUJU_ACTION_TAG=`+action.Tag().String()+`(\|.*|$)`)
}

func (s *FactorySuite) TestNewActionRunnerBadCharm(c *gc.C) {
	rnr, err := s.factory.NewActionRunner("irrelevant")
	c.Assert(rnr, gc.IsNil)
	c.Assert(errors.Cause(err), jc.Satisfies, os.IsNotExist)
	c.Assert(err, gc.Not(jc.Satisfies), runner.IsBadActionError)
}

func (s *FactorySuite) TestNewActionRunnerBadName(c *gc.C) {
	s.SetCharm(c, "dummy")
	action, err := s.State.EnqueueAction(s.unit.Tag(), "no-such-action", nil)
	c.Assert(err, jc.ErrorIsNil) // this will fail when using AddAction on unit
	rnr, err := s.factory.NewActionRunner(action.Id())
	c.Check(rnr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot run \"no-such-action\" action: not defined")
	c.Check(err, jc.Satisfies, runner.IsBadActionError)
}

func (s *FactorySuite) TestNewActionRunnerBadParams(c *gc.C) {
	s.SetCharm(c, "dummy")
	action, err := s.State.EnqueueAction(s.unit.Tag(), "snapshot", map[string]interface{}{
		"outfile": 123,
	})
	c.Assert(err, jc.ErrorIsNil) // this will fail when state is done right
	rnr, err := s.factory.NewActionRunner(action.Id())
	c.Check(rnr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot run \"snapshot\" action: .*")
	c.Check(err, jc.Satisfies, runner.IsBadActionError)
}

func (s *FactorySuite) TestNewActionRunnerMissingAction(c *gc.C) {
	s.SetCharm(c, "dummy")
	action, err := s.State.EnqueueAction(s.unit.Tag(), "snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.unit.CancelAction(action)
	c.Assert(err, jc.ErrorIsNil)
	rnr, err := s.factory.NewActionRunner(action.Id())
	c.Check(rnr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "action no longer available")
	c.Check(err, gc.Equals, runner.ErrActionNotAvailable)
}

func (s *FactorySuite) TestNewActionRunnerUnauthAction(c *gc.C) {
	s.SetCharm(c, "dummy")
	otherUnit, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	action, err := s.State.EnqueueAction(otherUnit.Tag(), "snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)
	rnr, err := s.factory.NewActionRunner(action.Id())
	c.Check(rnr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "action no longer available")
	c.Check(err, gc.Equals, runner.ErrActionNotAvailable)
}
