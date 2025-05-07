// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"context"
	"reflect"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/proxy"
	"github.com/juju/tc"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	uniterapi "github.com/juju/juju/internal/worker/uniter/api"
	runnercontext "github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	runnertesting "github.com/juju/juju/internal/worker/uniter/runner/testing"
	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/rpc/params"
)

var noProxies = proxy.Settings{}
var apiAddrs = []string{"a1:123", "a2:123"}

type hookCommitMatcher struct {
	c        *tc.C
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
	unit     *uniterapi.MockUnit
	relunits map[int]*uniterapi.MockRelationUnit

	// Initial hook context data.
	machinePortRanges map[names.UnitTag]network.GroupedPortRanges
}

func (s *BaseHookContextSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.relunits = map[int]*uniterapi.MockRelationUnit{}
	s.secrets = &runnertesting.SecretsContextAccessor{}
	s.machinePortRanges = make(map[names.UnitTag]network.GroupedPortRanges)
	s.clock = testclock.NewClock(time.Time{})
}

func (s *BaseHookContextSuite) GetContext(c *tc.C, ctrl *gomock.Controller, relId int, remoteName string, storageTag names.StorageTag) jujuc.Context {
	uuid, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return s.getHookContext(c, ctrl, uuid.String(), relId, remoteName, storageTag)
}

func (s *BaseHookContextSuite) AddContextRelation(c *tc.C, ctrl *gomock.Controller, name string) {
	num := len(s.relunits)
	rel := uniterapi.NewMockRelation(ctrl)
	rel.EXPECT().Id().Return(num).AnyTimes()
	rel.EXPECT().Tag().Return(names.NewRelationTag("mysql:server wordpress:" + name)).AnyTimes()
	rel.EXPECT().Life().Return(life.Alive).AnyTimes()
	rel.EXPECT().Suspended().Return(false).AnyTimes()

	relUnit := uniterapi.NewMockRelationUnit(ctrl)
	relUnit.EXPECT().Relation().Return(rel).AnyTimes()
	relUnit.EXPECT().Endpoint().Return(apiuniter.Endpoint{Relation: charm.Relation{Name: "db"}}).AnyTimes()
	relUnit.EXPECT().Settings(gomock.Any()).Return(
		apiuniter.NewSettings(rel.Tag().String(), names.NewUnitTag("u/0").String(), params.Settings{}), nil,
	).AnyTimes()

	s.relunits[num] = relUnit
}

func (s *BaseHookContextSuite) setupUnit(ctrl *gomock.Controller) names.MachineTag {
	unitTag := names.NewUnitTag("u/0")
	s.unit = uniterapi.NewMockUnit(ctrl)
	s.unit.EXPECT().Tag().Return(unitTag).AnyTimes()
	s.unit.EXPECT().Name().Return(unitTag.Id()).AnyTimes()
	s.unit.EXPECT().PublicAddress(gomock.Any()).Return("u-0.testing.invalid", nil).AnyTimes()
	s.unit.EXPECT().PrivateAddress(gomock.Any()).Return("u-0.testing.invalid", nil).AnyTimes()
	s.unit.EXPECT().AvailabilityZone(gomock.Any()).Return("a-zone", nil).AnyTimes()

	machineTag := names.NewMachineTag("0")
	s.unit.EXPECT().AssignedMachine(gomock.Any()).Return(machineTag, nil).AnyTimes()
	return machineTag
}

func (s *BaseHookContextSuite) setupUniter(ctrl *gomock.Controller) names.MachineTag {
	machineTag := s.setupUnit(ctrl)
	s.uniter = uniterapi.NewMockUniterClient(ctrl)
	s.uniter.EXPECT().OpenedMachinePortRangesByEndpoint(gomock.Any(), machineTag).DoAndReturn(func(_ context.Context, _ names.MachineTag) (map[names.UnitTag]network.GroupedPortRanges, error) {
		return s.machinePortRanges, nil
	}).AnyTimes()
	s.uniter.EXPECT().OpenedPortRangesByEndpoint(gomock.Any()).Return(nil, nil).AnyTimes()
	return machineTag
}

func (s *BaseHookContextSuite) getHookContext(c *tc.C, ctrl *gomock.Controller, uuid string, relid int, remote string, storageTag names.StorageTag) *runnercontext.HookContext {
	if relid != -1 {
		_, found := s.relunits[relid]
		c.Assert(found, jc.IsTrue)
	}
	machineTag := s.setupUniter(ctrl)

	relctxs := map[int]*runnercontext.ContextRelation{}
	for relId, relUnit := range s.relunits {
		cache := runnercontext.NewRelationCache(relUnit.ReadSettings, nil)
		relctxs[relId] = runnercontext.NewContextRelation(relUnit, cache, false)
	}
	context, err := runnercontext.NewHookContext(c, runnercontext.HookContextParams{
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

func (s *BaseHookContextSuite) AssertCoreContext(c *tc.C, ctx *runnercontext.HookContext) {
	c.Assert(ctx.UnitName(), tc.Equals, "u/0")
	c.Assert(runnercontext.ContextMachineTag(ctx), jc.DeepEquals, names.NewMachineTag("0"))

	actual, err := ctx.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, tc.Equals, "u-0.testing.invalid")

	actual, err = ctx.PublicAddress(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, tc.Equals, "u-0.testing.invalid")

	name, uuid := runnercontext.ContextEnvInfo(ctx)
	c.Assert(name, tc.Equals, "test-model")
	c.Assert(uuid, tc.Equals, coretesting.ModelTag.Id())

	ids, err := ctx.RelationIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids, tc.HasLen, 2)

	r, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Name(), tc.Equals, "db")
	c.Assert(r.FakeId(), tc.Equals, "db:0")

	r, err = ctx.Relation(1)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Name(), tc.Equals, "db")
	c.Assert(r.FakeId(), tc.Equals, "db:1")

	az, err := ctx.AvailabilityZone()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(az, tc.Equals, "a-zone")

	info, err := ctx.SecretMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, tc.HasLen, 1)
	for id, v := range info {
		c.Assert(id, tc.Equals, "9m4e2mr0ui3e8a215n4g")
		c.Assert(v.Label, tc.Equals, "label")
		c.Assert(v.Owner, jc.DeepEquals, secrets.Owner{Kind: secrets.ApplicationOwner, ID: "mariadb"})
		c.Assert(v.Description, tc.Equals, "description")
		c.Assert(v.RotatePolicy, tc.Equals, secrets.RotateHourly)
		c.Assert(v.LatestRevision, tc.Equals, 666)
		c.Assert(v.LatestChecksum, tc.Equals, "deadbeef")
	}
}

func (s *BaseHookContextSuite) AssertNotActionContext(c *tc.C, ctx *runnercontext.HookContext) {
	actionData, err := ctx.ActionData()
	c.Assert(actionData, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "not running an action")
}

func (s *BaseHookContextSuite) AssertActionContext(c *tc.C, ctx *runnercontext.HookContext) {
	actionData, err := ctx.ActionData()
	c.Assert(actionData, tc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BaseHookContextSuite) AssertNotStorageContext(c *tc.C, ctx *runnercontext.HookContext) {
	storageAttachment, err := ctx.HookStorage(context.Background())
	c.Assert(storageAttachment, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, ".*")
}

func (s *BaseHookContextSuite) AssertStorageContext(c *tc.C, ctx *runnercontext.HookContext, id string, attachment storage.StorageAttachmentInfo) {
	fromCache, err := ctx.HookStorage(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fromCache, tc.NotNil)
	c.Assert(fromCache.Tag().Id(), tc.Equals, id)
	c.Assert(fromCache.Kind(), tc.Equals, attachment.Kind)
	c.Assert(fromCache.Location(), tc.Equals, attachment.Location)
}

func (s *BaseHookContextSuite) AssertRelationContext(c *tc.C, ctx *runnercontext.HookContext, relId int, remoteUnit string, remoteApp string) *runnercontext.ContextRelation {
	actualRemoteUnit, _ := ctx.RemoteUnitName()
	c.Assert(actualRemoteUnit, tc.Equals, remoteUnit)
	actualRemoteApp, _ := ctx.RemoteApplicationName()
	c.Assert(actualRemoteApp, tc.Equals, remoteApp)
	rel, err := ctx.HookRelation()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Id(), tc.Equals, relId)
	return rel.(*runnercontext.ContextRelation)
}

func (s *BaseHookContextSuite) AssertNotRelationContext(c *tc.C, ctx *runnercontext.HookContext) {
	rel, err := ctx.HookRelation()
	c.Assert(rel, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, ".*")
}

func (s *BaseHookContextSuite) AssertSecretContext(c *tc.C, ctx *runnercontext.HookContext, secretURI, label string, revision int) {
	uri, _ := ctx.SecretURI()
	c.Assert(uri, tc.Equals, secretURI)
	c.Assert(ctx.SecretLabel(), tc.Equals, label)
	c.Assert(ctx.SecretRevision(), tc.Equals, revision)
}

func (s *BaseHookContextSuite) AssertNotSecretContext(c *tc.C, ctx *runnercontext.HookContext) {
	workloadName, err := ctx.SecretURI()
	c.Assert(err, tc.NotNil)
	c.Assert(workloadName, tc.Equals, "")
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

func (MockEnvPaths) GetJujucClientSocket() sockets.Socket {
	return sockets.Socket{Network: "unix", Address: "path-to-jujuc.socket"}
}

func (MockEnvPaths) GetJujucServerSocket() sockets.Socket {
	return sockets.Socket{Network: "unix", Address: "path-to-jujuc.socket"}
}

func (MockEnvPaths) GetMetricsSpoolDir() string {
	return "path-to-metrics-spool-dir"
}
