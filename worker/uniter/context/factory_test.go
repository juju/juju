// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/fs"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
)

type FactorySuite struct {
	HookContextSuite
	paths      RealPaths
	factory    context.Factory
	membership map[int][]string
}

var _ = gc.Suite(&FactorySuite{})

func (s *FactorySuite) SetUpTest(c *gc.C) {
	s.HookContextSuite.SetUpTest(c)
	s.paths = NewRealPaths(c)
	s.membership = map[int][]string{}
	factory, err := context.NewFactory(
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

func (s *FactorySuite) getRelationInfos() map[int]*context.RelationInfo {
	info := map[int]*context.RelationInfo{}
	for relId, relUnit := range s.apiRelunits {
		info[relId] = &context.RelationInfo{
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

func (s *FactorySuite) updateCache(relId int, unitName string, settings params.RelationSettings) {
	context.UpdateCachedSettings(s.factory, relId, unitName, settings)
}

func (s *FactorySuite) getCache(relId int, unitName string) (params.RelationSettings, bool) {
	return context.CachedSettings(s.factory, relId, unitName)
}

func (s *FactorySuite) AssertPaths(c *gc.C, rnr context.Runner) {
	c.Assert(context.RunnerPaths(rnr), gc.DeepEquals, s.paths)
}

func (s *FactorySuite) AssertCoreContext(c *gc.C, ctx context.Context) {
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
	c.Assert(ctx.OwnerTag(), gc.Equals, s.service.GetOwnerTag())
	c.Assert(context.ContextMachineTag(ctx), jc.DeepEquals, names.NewMachineTag("0"))

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
	name, uuid := context.ContextEnvInfo(ctx)
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

func (s *FactorySuite) AssertNotActionContext(c *gc.C, ctx context.Context) {
	actionData, err := ctx.ActionData()
	c.Assert(actionData, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "not running an action")
}

func (s *FactorySuite) AssertRelationContext(c *gc.C, ctx context.Context, relId int) *context.ContextRelation {
	rel, found := ctx.HookRelation()
	c.Assert(found, jc.IsTrue)
	c.Assert(rel.Id(), gc.Equals, relId)
	return rel.(*context.ContextRelation)
}

func (s *FactorySuite) AssertNotRelationContext(c *gc.C, ctx context.Context) {
	rel, found := ctx.HookRelation()
	c.Assert(rel, gc.IsNil)
	c.Assert(found, jc.IsFalse)
}

func (s *FactorySuite) TestNewRunner(c *gc.C) {
	rnr, err := s.factory.NewRunner(-1, "")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
}

func (s *FactorySuite) TestNewRunnerRelationId(c *gc.C) {
	rnr, err := s.factory.NewRunner(0, "foo")
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertRelationContext(c, ctx, 0)
}

func (s *FactorySuite) TestNewRunnerRelationIdDoesNotExist(c *gc.C) {
	_, err := s.factory.NewRunner(12, "baz")
	c.Assert(err, gc.ErrorMatches, `unknown relation id:.*`)
}

func (s *FactorySuite) TestNewHookRunner(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{Kind: hooks.ConfigChanged})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
}

func (s *FactorySuite) TestNewHookRunnerWithBadHook(c *gc.C) {
	rnr, err := s.factory.NewHookRunner(hook.Info{})
	c.Assert(rnr, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, `unknown hook kind ""`)
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
	s.AssertRelationContext(c, ctx, 1)
}

func (s *FactorySuite) TestNewHookRunnerPrunesNonMemberCaches(c *gc.C) {

	// Write cached member settings for a member and a non-member.
	s.setUpCacheMethods(c)
	s.membership[0] = []string{"rel0/0"}
	s.updateCache(0, "rel0/0", params.RelationSettings{"keep": "me"})
	s.updateCache(0, "rel0/1", params.RelationSettings{"drop": "me"})

	rnr, err := s.factory.NewHookRunner(hook.Info{Kind: hooks.Install})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()

	settings0, found := s.getCache(0, "rel0/0")
	c.Assert(found, jc.IsTrue)
	c.Assert(settings0, jc.DeepEquals, params.RelationSettings{"keep": "me"})

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
	c.Assert(settings0, jc.DeepEquals, params.RelationSettings{"keep": "me"})

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
	s.updateCache(1, "r/0", params.RelationSettings{"foo": "bar"})

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
	rel := s.AssertRelationContext(c, ctx, 1)
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
	s.updateCache(1, "r/0", params.RelationSettings{"foo": "bar"})
	s.updateCache(1, "r/4", params.RelationSettings{"baz": "qux"})

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
	rel := s.AssertRelationContext(c, ctx, 1)
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0", "r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, jc.DeepEquals, params.RelationSettings{"foo": "bar"})
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
	s.updateCache(1, "r/0", params.RelationSettings{"foo": "bar"})
	s.updateCache(1, "r/4", params.RelationSettings{"baz": "qux"})

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
	rel := s.AssertRelationContext(c, ctx, 1)
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, gc.IsNil)
	c.Assert(member, jc.IsFalse)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, jc.DeepEquals, params.RelationSettings{"baz": "qux"})
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
	s.updateCache(1, "r/0", params.RelationSettings{"foo": "bar"})
	s.updateCache(1, "r/4", params.RelationSettings{"baz": "qux"})

	rnr, err := s.factory.NewHookRunner(hook.Info{
		Kind:       hooks.RelationBroken,
		RelationId: 1,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	rel := s.AssertRelationContext(c, ctx, 1)
	c.Assert(rel.UnitNames(), jc.DeepEquals, []string{"r/0", "r/4"})
	cached0, member := s.getCache(1, "r/0")
	c.Assert(cached0, jc.DeepEquals, params.RelationSettings{"foo": "bar"})
	c.Assert(member, jc.IsTrue)
	cached4, member := s.getCache(1, "r/4")
	c.Assert(cached4, jc.DeepEquals, params.RelationSettings{"baz": "qux"})
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

func (s *FactorySuite) TestNewActionRunnerBadCharm(c *gc.C) {
	rnr, err := s.factory.NewActionRunner("irrelevant")
	c.Assert(rnr, gc.IsNil)
	c.Assert(errors.Cause(err), jc.Satisfies, os.IsNotExist)
	c.Assert(err, gc.Not(jc.Satisfies), context.IsBadActionError)
}

func (s *FactorySuite) TestNewActionRunner(c *gc.C) {
	s.SetCharm(c, "dummy")
	action, err := s.unit.AddAction("snapshot", map[string]interface{}{
		"outfile": "/some/file.bz2",
	})
	c.Assert(err, jc.ErrorIsNil)
	rnr, err := s.factory.NewActionRunner(action.Id())
	c.Assert(err, jc.ErrorIsNil)
	s.AssertPaths(c, rnr)
	ctx := rnr.Context()
	data, err := ctx.ActionData()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(data, jc.DeepEquals, &context.ActionData{
		ActionName: "snapshot",
		ActionTag:  action.ActionTag(),
		ActionParams: map[string]interface{}{
			"outfile": "/some/file.bz2",
		},
		ResultsMap: map[string]interface{}{},
	})
}

func (s *FactorySuite) TestNewActionRunnerBadName(c *gc.C) {
	s.SetCharm(c, "dummy")
	action, err := s.unit.AddAction("no-such-action", nil)
	c.Assert(err, jc.ErrorIsNil) // this will fail when state is done right
	rnr, err := s.factory.NewActionRunner(action.Id())
	c.Check(rnr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot run \"no-such-action\" action: not defined")
	c.Check(err, jc.Satisfies, context.IsBadActionError)
}

func (s *FactorySuite) TestNewActionRunnerBadParams(c *gc.C) {
	s.SetCharm(c, "dummy")
	action, err := s.unit.AddAction("snapshot", map[string]interface{}{
		"outfile": 123,
	})
	c.Assert(err, jc.ErrorIsNil) // this will fail when state is done right
	rnr, err := s.factory.NewActionRunner(action.Id())
	c.Check(rnr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "cannot run \"snapshot\" action: .*")
	c.Check(err, jc.Satisfies, context.IsBadActionError)
}

func (s *FactorySuite) TestNewActionRunnerMissingAction(c *gc.C) {
	s.SetCharm(c, "dummy")
	action, err := s.unit.AddAction("snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.unit.CancelAction(action)
	c.Assert(err, jc.ErrorIsNil)
	rnr, err := s.factory.NewActionRunner(action.Id())
	c.Check(rnr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "action no longer available")
	c.Check(err, gc.Equals, context.ErrActionNotAvailable)
}

func (s *FactorySuite) TestNewActionRunnerUnauthAction(c *gc.C) {
	s.SetCharm(c, "dummy")
	otherUnit, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	action, err := otherUnit.AddAction("snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)
	rnr, err := s.factory.NewActionRunner(action.Id())
	c.Check(rnr, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "action no longer available")
	c.Check(err, gc.Equals, context.ErrActionNotAvailable)
}
