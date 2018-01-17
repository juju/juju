// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"runtime"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/status"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/caasoperator/commands"
	"github.com/juju/juju/worker/caasoperator/runner/context"
	"github.com/juju/juju/worker/caasoperator/runner/runnertesting"
)

var noProxies = proxy.Settings{}
var apiAddrs = []string{"a1:123", "a2:123"}

// HookContextSuite contains shared setup for various other test suites. Test
// methods should not be added to this type, because they'll get run repeatedly.
type HookContextSuite struct {
	testing.BaseSuite

	applicationName string
	relIdCounter    int
	clock           *jujutesting.Clock

	contextAPI   *runnertesting.MockContextAPI
	relationAPIs map[int]*runnertesting.MockRelationUnitAPI
}

func (s *HookContextSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("non windows functionality")
	}
	s.BaseSuite.SetUpSuite(c)
}

func (s *HookContextSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.relIdCounter = 0
	s.relationAPIs = make(map[int]*runnertesting.MockRelationUnitAPI)
	s.contextAPI = runnertesting.NewMockContextAPI(apiAddrs, proxy.Settings{})
	err := s.contextAPI.SetApplicationStatus(status.Maintenance, "initialising", nil)
	c.Assert(err, jc.ErrorIsNil)

	s.AddContextRelation(c, "db0")
	s.AddContextRelation(c, "db1")

	s.clock = jujutesting.NewClock(time.Time{})
}

func (s *HookContextSuite) GetContext(
	c *gc.C, relId int, remoteName string,
) commands.Context {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return s.getHookContext(
		c, uuid.String(), relId, remoteName, noProxies,
	)
}

func (s *HookContextSuite) AddContextRelation(c *gc.C, name string) {
	s.applicationName = name
	s.relationAPIs[s.relIdCounter] = runnertesting.NewMockRelationUnitAPI(s.relIdCounter, "db", false)
	s.relIdCounter++
}

func (s *HookContextSuite) getHookContext(c *gc.C, uuid string, relid int,
	remote string, proxies proxy.Settings) *context.HookContext {
	if relid != -1 {
		_, found := s.relationAPIs[relid]
		c.Assert(found, jc.IsTrue)
	}

	relctxs := map[int]*context.ContextRelation{}
	for relId, relUnit := range s.relationAPIs {
		cache := context.NewRelationCache(relUnit.RemoteSettings, nil)
		relctxs[relId] = context.NewContextRelation(relUnit, cache)
	}

	context, err := context.NewHookContext(s.contextAPI, "TestCtx", uuid, "gitlab",
		"gitlab-model", relid, remote, relctxs, apiAddrs,
		proxies, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *HookContextSuite) AssertCoreContext(c *gc.C, ctx *context.HookContext) {
	c.Assert(ctx.ApplicationName(), gc.Equals, "gitlab")

	name, uuid := context.ContextModelInfo(ctx)
	c.Assert(name, gc.Equals, "gitlab-model")
	c.Assert(uuid, gc.Equals, testing.ModelTag.Id())

	ids, err := ctx.RelationIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids, gc.HasLen, 2)

	r, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:0")

	r, err = ctx.Relation(1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:1")
}

func (s *HookContextSuite) AssertRelationContext(c *gc.C, ctx *context.HookContext, relId int, remoteUnit string) *context.ContextRelation {
	actualRemoteUnit, _ := ctx.RemoteUnitName()
	c.Assert(actualRemoteUnit, gc.Equals, remoteUnit)
	rel, err := ctx.HookRelation()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Id(), gc.Equals, relId)
	return rel.(*context.ContextRelation)
}

func (s *HookContextSuite) AssertNotRelationContext(c *gc.C, ctx *context.HookContext) {
	rel, err := ctx.HookRelation()
	c.Assert(rel, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, ".*")
}

// MockFakePaths implements Paths for tests that don't need to actually touch
// the filesystem.
type MockFakePaths struct{}

func (MockFakePaths) GetToolsDir() string {
	return "path-to-tools"
}

func (MockFakePaths) GetCharmDir() string {
	return "path-to-charm"
}

func (MockFakePaths) GetHookCommandSocket() string {
	return "path-to-hookcommand.socket"
}

func (MockFakePaths) GetMetricsSpoolDir() string {
	return "path-to-metrics-spool-dir"
}
