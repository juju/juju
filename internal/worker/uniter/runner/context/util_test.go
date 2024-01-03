// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"reflect"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/clock/testclock"
	"github.com/juju/names/v5"
	"github.com/juju/proxy"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/storage"
	uniterapi "github.com/juju/juju/internal/worker/uniter/api"
	runnercontext "github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	runnertesting "github.com/juju/juju/internal/worker/uniter/runner/testing"
	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var noProxies = proxy.Settings{}
var apiAddrs = []string{"a1:123", "a2:123"}

type hookCommitMatcher struct {
	c        *gc.C
	expected params.CommitHookChangesArgs
}

func (m hookCommitMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(params.CommitHookChangesArgs)
	if !ok {
		return false
	}

	if len(obtained.Args) != len(m.expected.Args) {
		return false
	}
	match := func(got, wanted params.CommitHookChangesArg) bool {
		if !m.c.Check(got.RelationUnitSettings, jc.SameContents, wanted.RelationUnitSettings) {
			return false
		}
		if !m.c.Check(got.AddStorage, jc.SameContents, wanted.AddStorage) {
			return false
		}
		if !m.c.Check(got.OpenPorts, jc.SameContents, wanted.OpenPorts) {
			return false
		}
		if !m.c.Check(got.ClosePorts, jc.SameContents, wanted.ClosePorts) {
			return false
		}
		if !m.c.Check(got.SecretCreates, jc.SameContents, wanted.SecretCreates) {
			return false
		}
		if !m.c.Check(got.SecretUpdates, jc.SameContents, wanted.SecretUpdates) {
			return false
		}
		if !m.c.Check(got.SecretDeletes, jc.SameContents, wanted.SecretDeletes) {
			return false
		}
		if !m.c.Check(got.SecretGrants, jc.SameContents, wanted.SecretGrants) {
			return false
		}
		if !m.c.Check(got.SecretRevokes, jc.SameContents, wanted.SecretRevokes) {
			return false
		}
		if got.UpdateNetworkInfo != wanted.UpdateNetworkInfo {
			return false
		}
		if !reflect.DeepEqual(got.SetUnitState, wanted.SetUnitState) {
			return false
		}
		return true
	}

	for i, a := range obtained.Args {
		if !match(a, m.expected.Args[i]) {
			return false
		}
	}
	return true
}

func (m hookCommitMatcher) String() string {
	return "Match the contents of the hook flush CommitHookChangesArgs arg"
}

// BaseHookContextSuite contains shared setup for various other test suites. Test
// methods should not be added to this type, because they'll get run repeatedly.
type BaseHookContextSuite struct {
	jujutesting.IsolationSuite
	secretMetadata map[string]jujuc.SecretMetadata
	secrets        *runnertesting.SecretsContextAccessor
	clock          *testclock.Clock

	uniter   *uniterapi.MockUniterClient
	payloads *mocks.MockPayloadAPIClient
	unit     *uniterapi.MockUnit
	relunits map[int]*uniterapi.MockRelationUnit

	// Initial hook context data.
	machinePortRanges map[names.UnitTag]network.GroupedPortRanges
}

func (s *BaseHookContextSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.relunits = map[int]*uniterapi.MockRelationUnit{}
	s.secrets = &runnertesting.SecretsContextAccessor{}
	s.machinePortRanges = make(map[names.UnitTag]network.GroupedPortRanges)
	s.clock = testclock.NewClock(time.Time{})
}

func (s *BaseHookContextSuite) GetContext(c *gc.C, ctrl *gomock.Controller, relId int, remoteName string, storageTag names.StorageTag) jujuc.Context {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return s.getHookContext(c, ctrl, uuid.String(), relId, remoteName, storageTag)
}

func (s *BaseHookContextSuite) AddContextRelation(c *gc.C, ctrl *gomock.Controller, name string) {
	num := len(s.relunits)
	rel := uniterapi.NewMockRelation(ctrl)
	rel.EXPECT().Id().Return(num).AnyTimes()
	rel.EXPECT().Tag().Return(names.NewRelationTag("mysql:server wordpress:" + name)).AnyTimes()

	relUnit := uniterapi.NewMockRelationUnit(ctrl)
	relUnit.EXPECT().Relation().Return(rel).AnyTimes()
	relUnit.EXPECT().Endpoint().Return(apiuniter.Endpoint{Relation: charm.Relation{Name: "db"}}).AnyTimes()
	relUnit.EXPECT().Settings().Return(
		apiuniter.NewSettings(rel.Tag().String(), names.NewUnitTag("u/0").String(), params.Settings{}), nil,
	).AnyTimes()

	s.relunits[num] = relUnit
}

func (s *BaseHookContextSuite) setupUnit(ctrl *gomock.Controller) names.MachineTag {
	unitTag := names.NewUnitTag("u/0")
	s.unit = uniterapi.NewMockUnit(ctrl)
	s.unit.EXPECT().Tag().Return(unitTag).AnyTimes()
	s.unit.EXPECT().Name().Return(unitTag.Id()).AnyTimes()
	s.unit.EXPECT().MeterStatus().Return("", "", nil).AnyTimes()
	s.unit.EXPECT().PublicAddress().Return("u-0.testing.invalid", nil).AnyTimes()
	s.unit.EXPECT().PrivateAddress().Return("u-0.testing.invalid", nil).AnyTimes()
	s.unit.EXPECT().AvailabilityZone().Return("a-zone", nil).AnyTimes()

	machineTag := names.NewMachineTag("0")
	s.unit.EXPECT().AssignedMachine().Return(machineTag, nil).AnyTimes()
	return machineTag
}

func (s *BaseHookContextSuite) setupUniter(ctrl *gomock.Controller) names.MachineTag {
	machineTag := s.setupUnit(ctrl)
	s.uniter = uniterapi.NewMockUniterClient(ctrl)
	s.uniter.EXPECT().OpenedMachinePortRangesByEndpoint(machineTag).DoAndReturn(func(_ names.MachineTag) (map[names.UnitTag]network.GroupedPortRanges, error) {
		return s.machinePortRanges, nil
	}).AnyTimes()
	s.uniter.EXPECT().OpenedPortRangesByEndpoint().Return(nil, nil).AnyTimes()
	return machineTag
}

func (s *BaseHookContextSuite) getHookContext(c *gc.C, ctrl *gomock.Controller, uuid string, relid int, remote string, storageTag names.StorageTag) *runnercontext.HookContext {
	if relid != -1 {
		_, found := s.relunits[relid]
		c.Assert(found, jc.IsTrue)
	}
	machineTag := s.setupUniter(ctrl)

	relctxs := map[int]*runnercontext.ContextRelation{}
	for relId, relUnit := range s.relunits {
		cache := runnercontext.NewRelationCache(relUnit.ReadSettings, nil)
		relctxs[relId] = runnercontext.NewContextRelation(relUnit, cache)
	}
	context, err := runnercontext.NewHookContext(runnercontext.HookContextParams{
		Unit:                s.unit,
		Uniter:              s.uniter,
		ID:                  "TestCtx",
		UUID:                uuid,
		ModelName:           "test-model",
		RelationID:          relid,
		RemoteUnitName:      remote,
		Relations:           relctxs,
		APIAddrs:            apiAddrs,
		LegacyProxySettings: noProxies,
		JujuProxySettings:   noProxies,
		CanAddMetrics:       false,
		CharmMetrics:        nil,
		ActionData:          nil,
		AssignedMachineTag:  machineTag,
		SecretMetadata:      s.secretMetadata,
		SecretsClient:       s.secrets,
		SecretsStore:        s.secrets,
		StorageTag:          storageTag,
		Paths:               runnertesting.NewRealPaths(c),
		Clock:               s.clock,
	})

	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *BaseHookContextSuite) getMeteredHookContext(c *gc.C, ctrl *gomock.Controller, uuid string, relid int,
	remote string, canAddMetrics bool, metrics *charm.Metrics, paths runnertesting.RealPaths) *runnercontext.HookContext {
	relctxs := map[int]*runnercontext.ContextRelation{}

	s.setupUniter(ctrl)

	context, err := runnercontext.NewHookContext(runnercontext.HookContextParams{
		Unit:                s.unit,
		Uniter:              s.uniter,
		ID:                  "TestCtx",
		UUID:                uuid,
		ModelName:           "test-model",
		RelationID:          relid,
		RemoteUnitName:      remote,
		Relations:           relctxs,
		APIAddrs:            apiAddrs,
		LegacyProxySettings: noProxies,
		JujuProxySettings:   noProxies,
		CanAddMetrics:       canAddMetrics,
		CharmMetrics:        metrics,
		ActionData:          nil,
		AssignedMachineTag:  names.NewMachineTag("0"),
		Paths:               paths,
		Clock:               s.clock,
	})
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *BaseHookContextSuite) metricsDefinition(name string) *charm.Metrics {
	return &charm.Metrics{Metrics: map[string]charm.Metric{name: {Type: charm.MetricTypeGauge, Description: "generated metric"}}}
}

func (s *BaseHookContextSuite) AssertCoreContext(c *gc.C, ctx *runnercontext.HookContext) {
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
	c.Assert(runnercontext.ContextMachineTag(ctx), jc.DeepEquals, names.NewMachineTag("0"))

	actual, err := ctx.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, gc.Equals, "u-0.testing.invalid")

	actual, err = ctx.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, gc.Equals, "u-0.testing.invalid")

	name, uuid := runnercontext.ContextEnvInfo(ctx)
	c.Assert(name, gc.Equals, "test-model")
	c.Assert(uuid, gc.Equals, coretesting.ModelTag.Id())

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

	az, err := ctx.AvailabilityZone()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(az, gc.Equals, "a-zone")

	info, err := ctx.SecretMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, gc.HasLen, 1)
	for id, v := range info {
		c.Assert(id, gc.Equals, "9m4e2mr0ui3e8a215n4g")
		c.Assert(v.Label, gc.Equals, "label")
		c.Assert(v.Owner.String(), gc.Equals, "application-mariadb")
		c.Assert(v.Description, gc.Equals, "description")
		c.Assert(v.RotatePolicy, gc.Equals, secrets.RotateHourly)
		c.Assert(v.LatestRevision, gc.Equals, 666)
	}
}

func (s *BaseHookContextSuite) AssertNotActionContext(c *gc.C, ctx *runnercontext.HookContext) {
	actionData, err := ctx.ActionData()
	c.Assert(actionData, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "not running an action")
}

func (s *BaseHookContextSuite) AssertActionContext(c *gc.C, ctx *runnercontext.HookContext) {
	actionData, err := ctx.ActionData()
	c.Assert(actionData, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BaseHookContextSuite) AssertNotStorageContext(c *gc.C, ctx *runnercontext.HookContext) {
	storageAttachment, err := ctx.HookStorage()
	c.Assert(storageAttachment, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, ".*")
}

func (s *BaseHookContextSuite) AssertStorageContext(c *gc.C, ctx *runnercontext.HookContext, id string, attachment storage.StorageAttachmentInfo) {
	fromCache, err := ctx.HookStorage()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fromCache, gc.NotNil)
	c.Assert(fromCache.Tag().Id(), gc.Equals, id)
	c.Assert(fromCache.Kind(), gc.Equals, attachment.Kind)
	c.Assert(fromCache.Location(), gc.Equals, attachment.Location)
}

func (s *BaseHookContextSuite) AssertRelationContext(c *gc.C, ctx *runnercontext.HookContext, relId int, remoteUnit string, remoteApp string) *runnercontext.ContextRelation {
	actualRemoteUnit, _ := ctx.RemoteUnitName()
	c.Assert(actualRemoteUnit, gc.Equals, remoteUnit)
	actualRemoteApp, _ := ctx.RemoteApplicationName()
	c.Assert(actualRemoteApp, gc.Equals, remoteApp)
	rel, err := ctx.HookRelation()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Id(), gc.Equals, relId)
	return rel.(*runnercontext.ContextRelation)
}

func (s *BaseHookContextSuite) AssertNotRelationContext(c *gc.C, ctx *runnercontext.HookContext) {
	rel, err := ctx.HookRelation()
	c.Assert(rel, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, ".*")
}

func (s *BaseHookContextSuite) AssertWorkloadContext(c *gc.C, ctx *runnercontext.HookContext, workloadName string) {
	actualWorkloadName, _ := ctx.WorkloadName()
	c.Assert(actualWorkloadName, gc.Equals, workloadName)
}

func (s *BaseHookContextSuite) AssertNotWorkloadContext(c *gc.C, ctx *runnercontext.HookContext) {
	workloadName, err := ctx.WorkloadName()
	c.Assert(err, gc.NotNil)
	c.Assert(workloadName, gc.Equals, "")
}

func (s *BaseHookContextSuite) AssertSecretContext(c *gc.C, ctx *runnercontext.HookContext, secretURI, label string, revision int) {
	uri, _ := ctx.SecretURI()
	c.Assert(uri, gc.Equals, secretURI)
	c.Assert(ctx.SecretLabel(), gc.Equals, label)
	c.Assert(ctx.SecretRevision(), gc.Equals, revision)
}

func (s *BaseHookContextSuite) AssertNotSecretContext(c *gc.C, ctx *runnercontext.HookContext) {
	workloadName, err := ctx.SecretURI()
	c.Assert(err, gc.NotNil)
	c.Assert(workloadName, gc.Equals, "")
}

// MockEnvPaths implements Paths for tests that don't need to actually touch
// the filesystem.
type MockEnvPaths struct{}

func (MockEnvPaths) GetToolsDir() string {
	return "path-to-tools"
}

func (MockEnvPaths) GetCharmDir() string {
	return "path-to-charm"
}

func (MockEnvPaths) GetResourcesDir() string {
	return "path-to-resources"
}

func (MockEnvPaths) GetBaseDir() string {
	return "path-to-base"
}

func (MockEnvPaths) GetJujucClientSocket(remote bool) sockets.Socket {
	if remote {
		return sockets.Socket{Network: "tcp", Address: "127.0.0.1:32000"}
	}
	return sockets.Socket{Network: "unix", Address: "path-to-jujuc.socket"}
}

func (MockEnvPaths) GetJujucServerSocket(remote bool) sockets.Socket {
	if remote {
		return sockets.Socket{Network: "tcp", Address: "127.0.0.1:32000"}
	}
	return sockets.Socket{Network: "unix", Address: "path-to-jujuc.socket"}
}

func (MockEnvPaths) GetMetricsSpoolDir() string {
	return "path-to-metrics-spool-dir"
}

type stubLeadershipSettingsAccessor struct {
	results map[string]string
}

func (s *stubLeadershipSettingsAccessor) Read(_ string) (result map[string]string, _ error) {
	return result, nil
}

func (s *stubLeadershipSettingsAccessor) Merge(_, _ string, settings map[string]string) error {
	if s.results == nil {
		s.results = make(map[string]string)
	}
	for k, v := range settings {
		s.results[k] = v
	}
	return nil
}
