// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/fs"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5/hooks"

	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner"
)

type ContextFactorySuite struct {
	HookContextSuite
	paths      RealPaths
	factory    runner.ContextFactory
	membership map[int][]string
}

var _ = gc.Suite(&ContextFactorySuite{})

type fakeTracker struct {
	leadership.Tracker
}

func (fakeTracker) ServiceName() string {
	return "service-name"
}

func (s *ContextFactorySuite) SetUpTest(c *gc.C) {
	s.HookContextSuite.SetUpTest(c)
	s.paths = NewRealPaths(c)
	s.membership = map[int][]string{}

	contextFactory, err := runner.NewContextFactory(
		s.uniter,
		s.unit.Tag().(names.UnitTag),
		fakeTracker{},
		s.getRelationInfos,
		s.storage,
		s.paths,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.factory = contextFactory
}

func (s *ContextFactorySuite) SetCharm(c *gc.C, name string) {
	err := os.RemoveAll(s.paths.charm)
	c.Assert(err, jc.ErrorIsNil)
	err = fs.Copy(testcharms.Repo.CharmDirPath(name), s.paths.charm)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ContextFactorySuite) getRelationInfos() map[int]*runner.RelationInfo {
	info := map[int]*runner.RelationInfo{}
	for relId, relUnit := range s.apiRelunits {
		info[relId] = &runner.RelationInfo{
			RelationUnit: relUnit,
			MemberNames:  s.membership[relId],
		}
	}
	return info
}

func (s *ContextFactorySuite) testLeadershipContextWiring(c *gc.C, createContext func() runner.Context) {
	var stub testing.Stub
	stub.SetErrors(errors.New("bam"))
	restore := runner.PatchNewLeadershipContext(
		func(accessor runner.LeadershipSettingsAccessor, tracker leadership.Tracker) runner.LeadershipContext {
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
		Args:     []interface{}{s.uniter.LeadershipSettings, fakeTracker{}},
	}, {
		FuncName: "IsLeader",
	}})

}

func (s *ContextFactorySuite) TestNewHookRunnerLeadershipContext(c *gc.C) {
	s.testLeadershipContextWiring(c, func() runner.Context {
		ctx, err := s.factory.HookContext(hook.Info{Kind: hooks.ConfigChanged})
		c.Assert(err, jc.ErrorIsNil)
		return ctx
	})
}

func (s *ContextFactorySuite) TestNewCommandRunnerLeadershipContext(c *gc.C) {
	s.testLeadershipContextWiring(c, func() runner.Context {
		ctx, err := s.factory.CommandContext(runner.CommandInfo{RelationId: -1})
		c.Assert(err, jc.ErrorIsNil)
		return ctx
	})
}

func (s *ContextFactorySuite) TestNewActionRunnerLeadershipContext(c *gc.C) {
	s.testLeadershipContextWiring(c, func() runner.Context {
		s.SetCharm(c, "dummy")
		action, err := s.State.EnqueueAction(s.unit.Tag(), "snapshot", nil)
		c.Assert(err, jc.ErrorIsNil)

		actionData := &runner.ActionData{
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

func (s *ContextFactorySuite) TestMetricsHookContext(c *gc.C) {
	s.SetCharm(c, "metered")
	hi := hook.Info{Kind: hooks.CollectMetrics}
	ctx, err := s.factory.HookContext(hi)
	c.Assert(err, jc.ErrorIsNil)

	err = ctx.AddMetric("pings", "1", time.Now())
	c.Assert(err, jc.ErrorIsNil)

	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
}

func (s *ContextFactorySuite) TestActionContext(c *gc.C) {
	s.SetCharm(c, "dummy")
	action, err := s.State.EnqueueAction(s.unit.Tag(), "snapshot", nil)
	c.Assert(err, jc.ErrorIsNil)

	actionData := &runner.ActionData{
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
	ctx, err := s.factory.CommandContext(runner.CommandInfo{RelationId: -1})
	c.Assert(err, jc.ErrorIsNil)

	s.AssertCoreContext(c, ctx)
	s.AssertNotActionContext(c, ctx)
	s.AssertNotRelationContext(c, ctx)
	s.AssertNotStorageContext(c, ctx)
}

type StubLeadershipContext struct {
	runner.LeadershipContext
	*testing.Stub
}

func (stub *StubLeadershipContext) IsLeader() (bool, error) {
	stub.MethodCall(stub, "IsLeader")
	return false, stub.NextErr()
}
