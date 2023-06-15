// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"strings"
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/clock/testclock"
	"github.com/juju/names/v4"
	"github.com/juju/proxy"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/client/block"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	runnercontext "github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	runnertesting "github.com/juju/juju/worker/uniter/runner/testing"
)

var noProxies = proxy.Settings{}
var apiAddrs = []string{"a1:123", "a2:123"}

// HookContextSuite contains shared setup for various other test suites. Test
// methods should not be added to this type, because they'll get run repeatedly.
type HookContextSuite struct {
	testing.JujuConnSuite
	application    *state.Application
	unit           *state.Unit
	machine        *state.Machine
	relCh          *state.Charm
	relUnits       map[int]*state.RelationUnit
	secretMetadata map[string]jujuc.SecretMetadata
	secrets        *runnertesting.SecretsContextAccessor
	clock          *testclock.Clock

	st             api.Connection
	uniter         *uniter.State
	payloads       *uniter.PayloadFacadeClient
	apiUnit        *uniter.Unit
	meteredAPIUnit *uniter.Unit
	meteredCharm   *state.Charm
	apiRelunits    map[int]*uniter.RelationUnit
	BlockHelper
}

func (s *HookContextSuite) SetUpTest(c *gc.C) {
	var err error
	s.JujuConnSuite.SetUpTest(c)
	s.BlockHelper = NewBlockHelper(s.APIState)
	c.Assert(s.BlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })

	// reset
	s.machine = nil

	sch := s.AddTestingCharm(c, "wordpress-nolimit")
	s.application = s.AddTestingApplication(c, "u", sch)
	s.unit = s.AddUnit(c, s.application)

	s.meteredCharm = s.AddTestingCharm(c, "metered")
	meteredApplication := s.AddTestingApplication(c, "m", s.meteredCharm)
	meteredUnit := s.addUnit(c, meteredApplication)

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAs(c, s.unit.Tag(), password)
	s.uniter, err = uniter.NewFromConnection(s.st)
	c.Assert(err, jc.ErrorIsNil)
	s.payloads = uniter.NewPayloadFacadeClient(s.st)
	c.Assert(s.uniter, gc.NotNil)
	s.apiUnit, err = s.uniter.Unit(s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	err = meteredUnit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	meteredState := s.OpenAPIAs(c, meteredUnit.Tag(), password)
	meteredUniter, err := uniter.NewFromConnection(meteredState)
	c.Assert(err, jc.ErrorIsNil)
	s.meteredAPIUnit, err = meteredUniter.Unit(meteredUnit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	// The unit must always have a charm URL set.
	// In theatre, this happens as part of the installation process,
	// which happens before the initial install hook.
	// We simulate that having happened by explicitly setting it here.
	//
	// The API is used instead of direct state access, because the API call
	// handles synchronisation with the cache where the data must reside for
	// config watching and retrieval to work.
	err = s.apiUnit.SetCharmURL(sch.String())
	c.Assert(err, jc.ErrorIsNil)
	err = s.meteredAPIUnit.SetCharmURL(s.meteredCharm.String())
	c.Assert(err, jc.ErrorIsNil)

	s.relCh = s.AddTestingCharm(c, "mysql")
	s.relUnits = map[int]*state.RelationUnit{}
	s.apiRelunits = map[int]*uniter.RelationUnit{}
	s.AddContextRelation(c, "db0")
	s.AddContextRelation(c, "db1")

	s.secrets = &runnertesting.SecretsContextAccessor{}

	s.clock = testclock.NewClock(time.Time{})
}

func (s *HookContextSuite) GetContext(c *gc.C, relId int, remoteName string, storageTag names.StorageTag) jujuc.Context {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return s.getHookContext(c, uuid.String(), relId, remoteName, storageTag)
}

func (s *HookContextSuite) addUnit(c *gc.C, app *state.Application) *state.Unit {
	unit, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	if s.machine != nil {
		err = unit.AssignToMachine(s.machine)
		c.Assert(err, jc.ErrorIsNil)
		return unit
	}

	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	machineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	s.machine, err = s.State.Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	zone := "a-zone"
	hwc := instance.HardwareCharacteristics{
		AvailabilityZone: &zone,
	}
	err = s.machine.SetProvisioned("i-exist", "", "fake_nonce", &hwc)
	c.Assert(err, jc.ErrorIsNil)
	return unit
}

func (s *HookContextSuite) AddUnit(c *gc.C, app *state.Application) *state.Unit {
	unit := s.addUnit(c, app)
	name := strings.Replace(unit.Name(), "/", "-", 1)
	privateAddr := network.NewSpaceAddress(name+".testing.invalid", network.WithScope(network.ScopeCloudLocal))
	err := s.machine.SetProviderAddresses(privateAddr)
	c.Assert(err, jc.ErrorIsNil)
	return unit
}

func (s *HookContextSuite) AddContextRelation(c *gc.C, name string) {
	s.AddTestingApplication(c, name, s.relCh)
	eps, err := s.State.InferEndpoints("u", name)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(map[string]interface{}{"relation-name": name})
	c.Assert(err, jc.ErrorIsNil)
	s.relUnits[rel.Id()] = ru
	apiRel, err := s.uniter.Relation(rel.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRelUnit, err := apiRel.Unit(s.apiUnit.Tag())
	c.Assert(err, jc.ErrorIsNil)
	s.apiRelunits[rel.Id()] = apiRelUnit
}

func (s *HookContextSuite) getHookContext(c *gc.C, uuid string, relid int, remote string, storageTag names.StorageTag) *runnercontext.HookContext {
	if relid != -1 {
		_, found := s.apiRelunits[relid]
		c.Assert(found, jc.IsTrue)
	}
	facade, err := uniter.NewFromConnection(s.st)
	c.Assert(err, jc.ErrorIsNil)

	relctxs := map[int]*runnercontext.ContextRelation{}
	for relId, relUnit := range s.apiRelunits {
		cache := runnercontext.NewRelationCache(relUnit.ReadSettings, nil)
		relctxs[relId] = runnercontext.NewContextRelation(&relUnitShim{relUnit}, cache)
	}

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	context, err := runnercontext.NewHookContext(runnercontext.HookContextParams{
		Unit:                s.apiUnit,
		State:               facade,
		ID:                  "TestCtx",
		UUID:                uuid,
		ModelName:           env.Name(),
		RelationID:          relid,
		RemoteUnitName:      remote,
		Relations:           relctxs,
		APIAddrs:            apiAddrs,
		LegacyProxySettings: noProxies,
		JujuProxySettings:   noProxies,
		CanAddMetrics:       false,
		CharmMetrics:        nil,
		ActionData:          nil,
		AssignedMachineTag:  s.machine.Tag().(names.MachineTag),
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

func (s *HookContextSuite) getMeteredHookContext(c *gc.C, uuid string, relid int,
	remote string, canAddMetrics bool, metrics *charm.Metrics, paths runnertesting.RealPaths) *runnercontext.HookContext {
	if relid != -1 {
		_, found := s.apiRelunits[relid]
		c.Assert(found, jc.IsTrue)
	}
	facade, err := uniter.NewFromConnection(s.st)
	c.Assert(err, jc.ErrorIsNil)

	relctxs := map[int]*runnercontext.ContextRelation{}
	for relId, relUnit := range s.apiRelunits {
		cache := runnercontext.NewRelationCache(relUnit.ReadSettings, nil)
		relctxs[relId] = runnercontext.NewContextRelation(&relUnitShim{relUnit}, cache)
	}

	context, err := runnercontext.NewHookContext(runnercontext.HookContextParams{
		Unit:                s.meteredAPIUnit,
		State:               facade,
		ID:                  "TestCtx",
		UUID:                uuid,
		ModelName:           "test-model-name",
		RelationID:          relid,
		RemoteUnitName:      remote,
		Relations:           relctxs,
		APIAddrs:            apiAddrs,
		LegacyProxySettings: noProxies,
		JujuProxySettings:   noProxies,
		CanAddMetrics:       canAddMetrics,
		CharmMetrics:        metrics,
		ActionData:          nil,
		AssignedMachineTag:  s.machine.Tag().(names.MachineTag),
		Paths:               paths,
		Clock:               s.clock,
	})
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *HookContextSuite) metricsDefinition(name string) *charm.Metrics {
	return &charm.Metrics{Metrics: map[string]charm.Metric{name: {Type: charm.MetricTypeGauge, Description: "generated metric"}}}
}

func (s *HookContextSuite) AssertCoreContext(c *gc.C, ctx *runnercontext.HookContext) {
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
	c.Assert(runnercontext.ContextMachineTag(ctx), jc.DeepEquals, names.NewMachineTag("0"))

	expect, expectErr := s.unit.PrivateAddress()
	actual, actualErr := ctx.PrivateAddress()
	c.Assert(actual, gc.Equals, expect.Value)
	c.Assert(actualErr, jc.DeepEquals, expectErr)

	expect, expectErr = s.unit.PublicAddress()
	actual, actualErr = ctx.PublicAddress()
	c.Assert(actual, gc.Equals, expect.Value)
	c.Assert(actualErr, jc.DeepEquals, expectErr)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	name, uuid := runnercontext.ContextEnvInfo(ctx)
	c.Assert(name, gc.Equals, env.Name())
	c.Assert(uuid, gc.Equals, env.UUID())

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

func (s *HookContextSuite) AssertNotActionContext(c *gc.C, ctx *runnercontext.HookContext) {
	actionData, err := ctx.ActionData()
	c.Assert(actionData, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "not running an action")
}

func (s *HookContextSuite) AssertActionContext(c *gc.C, ctx *runnercontext.HookContext) {
	actionData, err := ctx.ActionData()
	c.Assert(actionData, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HookContextSuite) AssertNotStorageContext(c *gc.C, ctx *runnercontext.HookContext) {
	storageAttachment, err := ctx.HookStorage()
	c.Assert(storageAttachment, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, ".*")
}

func (s *HookContextSuite) AssertStorageContext(c *gc.C, ctx *runnercontext.HookContext, id string, attachment storage.StorageAttachmentInfo) {
	fromCache, err := ctx.HookStorage()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fromCache, gc.NotNil)
	c.Assert(fromCache.Tag().Id(), gc.Equals, id)
	c.Assert(fromCache.Kind(), gc.Equals, attachment.Kind)
	c.Assert(fromCache.Location(), gc.Equals, attachment.Location)
}

func (s *HookContextSuite) AssertRelationContext(c *gc.C, ctx *runnercontext.HookContext, relId int, remoteUnit string, remoteApp string) *runnercontext.ContextRelation {
	actualRemoteUnit, _ := ctx.RemoteUnitName()
	c.Assert(actualRemoteUnit, gc.Equals, remoteUnit)
	actualRemoteApp, _ := ctx.RemoteApplicationName()
	c.Assert(actualRemoteApp, gc.Equals, remoteApp)
	rel, err := ctx.HookRelation()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Id(), gc.Equals, relId)
	return rel.(*runnercontext.ContextRelation)
}

func (s *HookContextSuite) AssertNotRelationContext(c *gc.C, ctx *runnercontext.HookContext) {
	rel, err := ctx.HookRelation()
	c.Assert(rel, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, ".*")
}

func (s *HookContextSuite) AssertWorkloadContext(c *gc.C, ctx *runnercontext.HookContext, workloadName string) {
	actualWorkloadName, _ := ctx.WorkloadName()
	c.Assert(actualWorkloadName, gc.Equals, workloadName)
}

func (s *HookContextSuite) AssertNotWorkloadContext(c *gc.C, ctx *runnercontext.HookContext) {
	workloadName, err := ctx.WorkloadName()
	c.Assert(err, gc.NotNil)
	c.Assert(workloadName, gc.Equals, "")
}

func (s *HookContextSuite) AssertSecretContext(c *gc.C, ctx *runnercontext.HookContext, secretURI, label string, revision int) {
	uri, _ := ctx.SecretURI()
	c.Assert(uri, gc.Equals, secretURI)
	c.Assert(ctx.SecretLabel(), gc.Equals, label)
	c.Assert(ctx.SecretRevision(), gc.Equals, revision)
}

func (s *HookContextSuite) AssertNotSecretContext(c *gc.C, ctx *runnercontext.HookContext) {
	workloadName, err := ctx.SecretURI()
	c.Assert(err, gc.NotNil)
	c.Assert(workloadName, gc.Equals, "")
}

type BlockHelper struct {
	blockClient *block.Client
}

// NewBlockHelper creates a block switch used in testing
// to manage desired juju blocks.
func NewBlockHelper(st api.Connection) BlockHelper {
	return BlockHelper{
		blockClient: block.NewClient(st),
	}
}

// on switches on desired block and
// asserts that no errors were encountered.
func (s *BlockHelper) on(c *gc.C, blockType model.BlockType, msg string) {
	c.Assert(s.blockClient.SwitchBlockOn(string(blockType), msg), gc.IsNil)
}

// BlockAllChanges switches changes block on.
// This prevents all changes to juju environment.
func (s *BlockHelper) BlockAllChanges(c *gc.C, msg string) {
	s.on(c, model.BlockChange, msg)
}

// BlockRemoveObject switches remove block on.
// This prevents any object/entity removal on juju environment
func (s *BlockHelper) BlockRemoveObject(c *gc.C, msg string) {
	s.on(c, model.BlockRemove, msg)
}

// BlockDestroyModel switches destroy block on.
// This prevents juju environment destruction.
func (s *BlockHelper) BlockDestroyModel(c *gc.C, msg string) {
	s.on(c, model.BlockDestroy, msg)
}

func (s *BlockHelper) Close() {
	s.blockClient.Close()
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
