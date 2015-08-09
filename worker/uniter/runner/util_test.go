// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/block"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

var noProxies = proxy.Settings{}
var apiAddrs = []string{"a1:123", "a2:123"}
var expectedApiAddrs = strings.Join(apiAddrs, " ")

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

// RealPaths implements Paths for tests that do touch the filesystem.
type RealPaths struct {
	tools        string
	charm        string
	socket       string
	metricsspool string
}

func osDependentSockPath(c *gc.C) string {
	sockPath := filepath.Join(c.MkDir(), "test.sock")
	if runtime.GOOS == "windows" {
		return `\\.\pipe` + sockPath[2:]
	}
	return sockPath
}

func NewRealPaths(c *gc.C) RealPaths {
	return RealPaths{
		tools:        c.MkDir(),
		charm:        c.MkDir(),
		socket:       osDependentSockPath(c),
		metricsspool: c.MkDir(),
	}
}

func (p RealPaths) GetMetricsSpoolDir() string {
	return p.metricsspool
}

func (p RealPaths) GetToolsDir() string {
	return p.tools
}

func (p RealPaths) GetCharmDir() string {
	return p.charm
}

func (p RealPaths) GetJujucSocket() string {
	return p.socket
}

// HookContextSuite contains shared setup for various other test suites. Test
// methods should not be added to this type, because they'll get run repeatedly.
type HookContextSuite struct {
	testing.JujuConnSuite
	service  *state.Service
	unit     *state.Unit
	machine  *state.Machine
	relch    *state.Charm
	relunits map[int]*state.RelationUnit
	storage  *storageContextAccessor

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
	s.storage = &storageContextAccessor{
		map[names.StorageTag]*contextStorage{
			storageData0: &contextStorage{
				storageData0,
				storage.StorageKindBlock,
				"/dev/sdb",
			},
		},
	}
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
	remote string, proxies proxy.Settings) *runner.HookContext {
	if relid != -1 {
		_, found := s.apiRelunits[relid]
		c.Assert(found, jc.IsTrue)
	}
	facade, err := s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)

	relctxs := map[int]*runner.ContextRelation{}
	for relId, relUnit := range s.apiRelunits {
		cache := runner.NewRelationCache(relUnit.ReadSettings, nil)
		relctxs[relId] = runner.NewContextRelation(relUnit, cache)
	}

	env, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)

	context, err := runner.NewHookContext(s.apiUnit, facade, "TestCtx", uuid,
		env.Name(), relid, remote, relctxs, apiAddrs,
		proxies, false, nil, nil, s.machine.Tag().(names.MachineTag),
		NewRealPaths(c))
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *HookContextSuite) getMeteredHookContext(c *gc.C, uuid string, relid int,
	remote string, proxies proxy.Settings, canAddMetrics bool, metrics *charm.Metrics, paths RealPaths) *runner.HookContext {
	if relid != -1 {
		_, found := s.apiRelunits[relid]
		c.Assert(found, jc.IsTrue)
	}
	facade, err := s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)

	relctxs := map[int]*runner.ContextRelation{}
	for relId, relUnit := range s.apiRelunits {
		cache := runner.NewRelationCache(relUnit.ReadSettings, nil)
		relctxs[relId] = runner.NewContextRelation(relUnit, cache)
	}

	context, err := runner.NewHookContext(s.meteredApiUnit, facade, "TestCtx", uuid,
		"test-env-name", relid, remote, relctxs, apiAddrs,
		proxies, canAddMetrics, metrics, nil, s.machine.Tag().(names.MachineTag),
		paths)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *HookContextSuite) metricsDefinition(name string) *charm.Metrics {
	return &charm.Metrics{Metrics: map[string]charm.Metric{name: {Type: charm.MetricTypeGauge, Description: "generated metric"}}}
}

func (s *HookContextSuite) AssertCoreContext(c *gc.C, ctx runner.Context) {
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
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

func (s *HookContextSuite) AssertNotActionContext(c *gc.C, ctx runner.Context) {
	actionData, err := ctx.ActionData()
	c.Assert(actionData, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "not running an action")
}

func (s *HookContextSuite) AssertActionContext(c *gc.C, ctx runner.Context) {
	actionData, err := ctx.ActionData()
	c.Assert(actionData, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HookContextSuite) AssertNotStorageContext(c *gc.C, ctx runner.Context) {
	storageAttachment, ok := ctx.HookStorage()
	c.Assert(storageAttachment, gc.IsNil)
	c.Assert(ok, jc.IsFalse)
}

func (s *HookContextSuite) AssertStorageContext(c *gc.C, ctx runner.Context, id string, attachment storage.StorageAttachmentInfo) {
	fromCache, ok := ctx.HookStorage()
	c.Assert(ok, jc.IsTrue)
	c.Assert(fromCache, gc.NotNil)
	c.Assert(fromCache.Tag().Id(), gc.Equals, id)
	c.Assert(fromCache.Kind(), gc.Equals, attachment.Kind)
	c.Assert(fromCache.Location(), gc.Equals, attachment.Location)
}

func (s *HookContextSuite) AssertRelationContext(c *gc.C, ctx runner.Context, relId int, remoteUnit string) *runner.ContextRelation {
	actualRemoteUnit, _ := ctx.RemoteUnitName()
	c.Assert(actualRemoteUnit, gc.Equals, remoteUnit)
	rel, found := ctx.HookRelation()
	c.Assert(found, jc.IsTrue)
	c.Assert(rel.Id(), gc.Equals, relId)
	return rel.(*runner.ContextRelation)
}

func (s *HookContextSuite) AssertNotRelationContext(c *gc.C, ctx runner.Context) {
	rel, found := ctx.HookRelation()
	c.Assert(rel, gc.IsNil)
	c.Assert(found, jc.IsFalse)
}

// hookSpec supports makeCharm.
type hookSpec struct {
	// dir is the directory to create the hook in.
	dir string
	// name is the name of the hook.
	name string
	// perm is the file permissions of the hook.
	perm os.FileMode
	// code is the exit status of the hook.
	code int
	// stdout holds a string to print to stdout
	stdout string
	// stderr holds a string to print to stderr
	stderr string
	// background holds a string to print in the background after 0.2s.
	background string
}

// makeCharm constructs a fake charm dir containing a single named hook
// with permissions perm and exit code code. If output is non-empty,
// the charm will write it to stdout and stderr, with each one prefixed
// by name of the stream.
func makeCharm(c *gc.C, spec hookSpec, charmDir string) {
	dir := charmDir
	if spec.dir != "" {
		dir = filepath.Join(dir, spec.dir)
		err := os.Mkdir(dir, 0755)
		c.Assert(err, jc.ErrorIsNil)
	}
	c.Logf("openfile perm %v", spec.perm)
	hook, err := os.OpenFile(
		filepath.Join(dir, spec.name), os.O_CREATE|os.O_WRONLY, spec.perm,
	)
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Assert(hook.Close(), gc.IsNil)
	}()

	printf := func(f string, a ...interface{}) {
		_, err := fmt.Fprintf(hook, f+"\n", a...)
		c.Assert(err, jc.ErrorIsNil)
	}
	if runtime.GOOS != "windows" {
		printf("#!/bin/bash")
	}
	printf(echoPidScript)
	if spec.stdout != "" {
		printf("echo %s", spec.stdout)
	}
	if spec.stderr != "" {
		printf("echo %s >&2", spec.stderr)
	}
	if spec.background != "" {
		// Print something fairly quickly, then sleep for
		// quite a long time - if the hook execution is
		// blocking because of the background process,
		// the hook execution will take much longer than
		// expected.
		printf("(sleep 0.2; echo %s; sleep 10) &", spec.background)
	}
	printf("exit %d", spec.code)
}

type storageContextAccessor struct {
	storage map[names.StorageTag]*contextStorage
}

func (s *storageContextAccessor) StorageTags() []names.StorageTag {
	tags := set.NewTags()
	for tag := range s.storage {
		tags.Add(tag)
	}
	storageTags := make([]names.StorageTag, len(tags))
	for i, tag := range tags.SortedValues() {
		storageTags[i] = tag.(names.StorageTag)
	}
	return storageTags
}

func (s *storageContextAccessor) Storage(tag names.StorageTag) (jujuc.ContextStorageAttachment, bool) {
	storage, ok := s.storage[tag]
	return storage, ok
}

type contextStorage struct {
	tag      names.StorageTag
	kind     storage.StorageKind
	location string
}

func (c *contextStorage) Tag() names.StorageTag {
	return c.tag
}

func (c *contextStorage) Kind() storage.StorageKind {
	return c.kind
}

func (c *contextStorage) Location() string {
	return c.location
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

// BlockDestroyEnvironment switches destroy block on.
// This prevents juju environment destruction.
func (s *BlockHelper) BlockDestroyEnvironment(c *gc.C, msg string) {
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

var _ runner.MetricsRecorder = (*StubMetricsRecorder)(nil)
