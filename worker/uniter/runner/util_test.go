// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/proxy"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/uniter/runner"
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

// RealPaths implements Paths for tests that do touch the filesystem.
type RealPaths struct {
	tools  string
	charm  string
	socket string
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
		tools:  c.MkDir(),
		charm:  c.MkDir(),
		socket: osDependentSockPath(c),
	}
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

	st             *api.State
	uniter         *uniter.State
	apiUnit        *uniter.Unit
	meteredApiUnit *uniter.Unit
	apiRelunits    map[int]*uniter.RelationUnit
}

func (s *HookContextSuite) SetUpTest(c *gc.C) {
	var err error
	s.JujuConnSuite.SetUpTest(c)

	// reset
	s.machine = nil

	sch := s.AddTestingCharm(c, "wordpress")
	s.service = s.AddTestingService(c, "u", sch)
	s.unit = s.AddUnit(c, s.service)

	meteredCharm := s.AddTestingCharm(c, "metered")
	meteredService := s.AddTestingService(c, "m", meteredCharm)
	meteredUnit := s.addUnit(c, meteredService)
	err = meteredUnit.SetCharmURL(meteredCharm.URL())
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
	privateAddr := network.NewAddress(name+".testing.invalid", network.ScopeCloudLocal)
	err := s.machine.SetAddresses(privateAddr)
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

	context, err := runner.NewHookContext(s.apiUnit, facade, "TestCtx", uuid,
		"test-env-name", relid, remote, relctxs, apiAddrs, names.NewUserTag("owner"),
		proxies, false, nil, nil, s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *HookContextSuite) getMeteredHookContext(c *gc.C, uuid string, relid int,
	remote string, proxies proxy.Settings, canAddMetrics bool, metrics *charm.Metrics) *runner.HookContext {
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
		"test-env-name", relid, remote, relctxs, apiAddrs, names.NewUserTag("owner"),
		proxies, canAddMetrics, metrics, nil, s.machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *HookContextSuite) metricsDefinition(name string) *charm.Metrics {
	return &charm.Metrics{Metrics: map[string]charm.Metric{name: {Type: charm.MetricTypeGauge, Description: "generated metric"}}}
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
