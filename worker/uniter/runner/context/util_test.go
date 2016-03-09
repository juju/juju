// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/block"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	runnertesting "github.com/juju/juju/worker/uniter/runner/testing"
)

var noProxies = proxy.Settings{}
var apiAddrs = []string{"a1:123", "a2:123"}
var expectedApiAddrs = strings.Join(apiAddrs, " ")

// HookContextSuite contains shared setup for various other test suites. Test
// methods should not be added to this type, because they'll get run repeatedly.
type HookContextSuite struct {
	testing.JujuConnSuite
	service  *state.Service
	unit     *state.Unit
	machine  *state.Machine
	relch    *state.Charm
	relunits map[int]*state.RelationUnit
	storage  *runnertesting.StorageContextAccessor
	clock    *coretesting.Clock

	st             api.Connection
	uniter         *uniter.State
	apiUnit        *uniter.Unit
	meteredApiUnit *uniter.Unit
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

	sch := s.AddTestingCharm(c, "wordpress")
	s.service = s.AddTestingService(c, "u", sch)
	s.unit = s.AddUnit(c, s.service)

	s.meteredCharm = s.AddTestingCharm(c, "metered")
	meteredService := s.AddTestingService(c, "m", s.meteredCharm)
	meteredUnit := s.addUnit(c, meteredService)
	err = meteredUnit.SetCharmURL(s.meteredCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	password, err := utils.RandomPassword()
	err = s.unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAs(c, s.unit.Tag(), password)
	s.uniter, err = s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.uniter, gc.NotNil)
	s.apiUnit, err = s.uniter.Unit(s.unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	err = meteredUnit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	meteredState := s.OpenAPIAs(c, meteredUnit.Tag(), password)
	meteredUniter, err := meteredState.Uniter()
	s.meteredApiUnit, err = meteredUniter.Unit(meteredUnit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	// Note: The unit must always have a charm URL set, because this
	// happens as part of the installation process (that happens
	// before the initial install hook).
	err = s.unit.SetCharmURL(sch.URL())
	c.Assert(err, jc.ErrorIsNil)
	s.relch = s.AddTestingCharm(c, "mysql")
	s.relunits = map[int]*state.RelationUnit{}
	s.apiRelunits = map[int]*uniter.RelationUnit{}
	s.AddContextRelation(c, "db0")
	s.AddContextRelation(c, "db1")

	storageData0 := names.NewStorageTag("data/0")
	s.storage = &runnertesting.StorageContextAccessor{
		map[names.StorageTag]*runnertesting.ContextStorage{
			storageData0: &runnertesting.ContextStorage{
				storageData0,
				storage.StorageKindBlock,
				"/dev/sdb",
			},
		},
	}

	s.clock = coretesting.NewClock(time.Time{})
}

func (s *HookContextSuite) GetContext(
	c *gc.C, relId int, remoteName string,
) jujuc.Context {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	return s.getHookContext(
		c, uuid.String(), relId, remoteName, noProxies,
	)
}

func (s *HookContextSuite) addUnit(c *gc.C, svc *state.Service) *state.Unit {
	unit, err := svc.AddUnit()
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
	err = s.machine.SetProvisioned("i-exist", "fake_nonce", &hwc)
	c.Assert(err, jc.ErrorIsNil)
	return unit
}

func (s *HookContextSuite) AddUnit(c *gc.C, svc *state.Service) *state.Unit {
	unit := s.addUnit(c, svc)
	name := strings.Replace(unit.Name(), "/", "-", 1)
	privateAddr := network.NewScopedAddress(name+".testing.invalid", network.ScopeCloudLocal)
	err := s.machine.SetProviderAddresses(privateAddr)
	c.Assert(err, jc.ErrorIsNil)
	return unit
}

func (s *HookContextSuite) AddContextRelation(c *gc.C, name string) {
	s.AddTestingService(c, name, s.relch)
	eps, err := s.State.InferEndpoints("u", name)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(s.unit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(map[string]interface{}{"relation-name": name})
	c.Assert(err, jc.ErrorIsNil)
	s.relunits[rel.Id()] = ru
	apiRel, err := s.uniter.Relation(rel.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRelUnit, err := apiRel.Unit(s.apiUnit)
	c.Assert(err, jc.ErrorIsNil)
	s.apiRelunits[rel.Id()] = apiRelUnit
}

func (s *HookContextSuite) getHookContext(c *gc.C, uuid string, relid int,
	remote string, proxies proxy.Settings) *context.HookContext {
	if relid != -1 {
		_, found := s.apiRelunits[relid]
		c.Assert(found, jc.IsTrue)
	}
	facade, err := s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)

	relctxs := map[int]*context.ContextRelation{}
	for relId, relUnit := range s.apiRelunits {
		cache := context.NewRelationCache(relUnit.ReadSettings, nil)
		relctxs[relId] = context.NewContextRelation(relUnit, cache)
	}

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	context, err := context.NewHookContext(s.apiUnit, facade, "TestCtx", uuid,
		env.Name(), relid, remote, relctxs, apiAddrs,
		proxies, false, nil, nil, s.machine.Tag().(names.MachineTag),
		runnertesting.NewRealPaths(c), s.clock)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *HookContextSuite) getMeteredHookContext(c *gc.C, uuid string, relid int,
	remote string, proxies proxy.Settings, canAddMetrics bool, metrics *charm.Metrics, paths runnertesting.RealPaths) *context.HookContext {
	if relid != -1 {
		_, found := s.apiRelunits[relid]
		c.Assert(found, jc.IsTrue)
	}
	facade, err := s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)

	relctxs := map[int]*context.ContextRelation{}
	for relId, relUnit := range s.apiRelunits {
		cache := context.NewRelationCache(relUnit.ReadSettings, nil)
		relctxs[relId] = context.NewContextRelation(relUnit, cache)
	}

	context, err := context.NewHookContext(s.meteredApiUnit, facade, "TestCtx", uuid,
		"test-model-name", relid, remote, relctxs, apiAddrs,
		proxies, canAddMetrics, metrics, nil, s.machine.Tag().(names.MachineTag),
		paths, s.clock)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *HookContextSuite) metricsDefinition(name string) *charm.Metrics {
	return &charm.Metrics{Metrics: map[string]charm.Metric{name: {Type: charm.MetricTypeGauge, Description: "generated metric"}}}
}

func (s *HookContextSuite) AssertCoreContext(c *gc.C, ctx *context.HookContext) {
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
	c.Assert(context.ContextMachineTag(ctx), jc.DeepEquals, names.NewMachineTag("0"))

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
	name, uuid := context.ContextEnvInfo(ctx)
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
}

func (s *HookContextSuite) AssertNotActionContext(c *gc.C, ctx *context.HookContext) {
	actionData, err := ctx.ActionData()
	c.Assert(actionData, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "not running an action")
}

func (s *HookContextSuite) AssertActionContext(c *gc.C, ctx *context.HookContext) {
	actionData, err := ctx.ActionData()
	c.Assert(actionData, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HookContextSuite) AssertNotStorageContext(c *gc.C, ctx *context.HookContext) {
	storageAttachment, err := ctx.HookStorage()
	c.Assert(storageAttachment, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, ".*")
}

func (s *HookContextSuite) AssertStorageContext(c *gc.C, ctx *context.HookContext, id string, attachment storage.StorageAttachmentInfo) {
	fromCache, err := ctx.HookStorage()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fromCache, gc.NotNil)
	c.Assert(fromCache.Tag().Id(), gc.Equals, id)
	c.Assert(fromCache.Kind(), gc.Equals, attachment.Kind)
	c.Assert(fromCache.Location(), gc.Equals, attachment.Location)
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
func (s *BlockHelper) on(c *gc.C, blockType multiwatcher.BlockType, msg string) {
	c.Assert(s.blockClient.SwitchBlockOn(string(blockType), msg), gc.IsNil)
}

// BlockAllChanges switches changes block on.
// This prevents all changes to juju environment.
func (s *BlockHelper) BlockAllChanges(c *gc.C, msg string) {
	s.on(c, multiwatcher.BlockChange, msg)
}

// BlockRemoveObject switches remove block on.
// This prevents any object/entity removal on juju environment
func (s *BlockHelper) BlockRemoveObject(c *gc.C, msg string) {
	s.on(c, multiwatcher.BlockRemove, msg)
}

// BlockDestroyModel switches destroy block on.
// This prevents juju environment destruction.
func (s *BlockHelper) BlockDestroyModel(c *gc.C, msg string) {
	s.on(c, multiwatcher.BlockDestroy, msg)
}

func (s *BlockHelper) Close() {
	s.blockClient.Close()
}

// StubMetricsRecorder implements the MetricsRecorder interface.
type StubMetricsRecorder struct {
	*jujutesting.Stub
}

// AddMetric implements the MetricsRecorder interface.
func (s StubMetricsRecorder) AddMetric(key, value string, created time.Time) error {
	s.AddCall("AddMetric", key, value, created)
	return nil
}

func (mr *StubMetricsRecorder) IsDeclaredMetric(key string) bool {
	mr.MethodCall(mr, "IsDeclaredMetric", key)
	return true
}

// Close implements the MetricsRecorder interface.
func (s StubMetricsRecorder) Close() error {
	s.AddCall("Close")
	return nil
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

func (MockEnvPaths) GetJujucSocket() string {
	return "path-to-jujuc.socket"
}

func (MockEnvPaths) GetMetricsSpoolDir() string {
	return "path-to-metrics-spool-dir"
}

func (MockEnvPaths) ComponentDir(name string) string {
	return filepath.Join("path-to-base-dir", name)
}
