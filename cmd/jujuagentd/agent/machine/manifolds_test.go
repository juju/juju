// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/dependency"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujuagentd/agent/machine"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/upgrades"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/internal/worker/gate"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

func TestManifoldsSuite(t *stdtesting.T) {
	tc.Run(t, &ManifoldsSuite{})
}

func (s *ManifoldsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *ManifoldsSuite) TestStartFuncsIAAS(c *tc.C) {
	s.assertStartFuncs(c, machine.IAASManifolds(machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	}))
}

func (s *ManifoldsSuite) TestStartFuncsCAAS(c *tc.C) {
	s.assertStartFuncs(c, machine.CAASManifolds(machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	}))
}

func (s *ManifoldsSuite) TestDependencyGraphsAreAcyclic(c *tc.C) {
	config := machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	}
	for _, manifolds := range []dependency.Manifolds{
		machine.IAASManifolds(config),
		machine.CAASManifolds(config),
	} {
		c.Check(dependency.Validate(manifolds), tc.ErrorIsNil)
	}
}

func (*ManifoldsSuite) assertStartFuncs(c *tc.C, manifolds dependency.Manifolds) {
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, tc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNamesIAAS(c *tc.C) {
	s.assertManifoldNames(
		c,
		machine.IAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		[]string{
			"agent",
			"api-address-updater",
			"api-caller",
			"api-config-watcher",
			"broker-tracker",
			"clock",
			"controller-agent-config",
			"controller-agent-config-ready-flag",
			"controller-agent-config-ready-gate",
			"deployer",
			"disk-manager",
			"flight-recorder",
			"host-key-reporter",
			"http-client",
			"log-router",
			"logging-config-updater",
			"loki-endpoint-updater",
			"lxd-container-provisioner",
			"machine-action-runner",
			"machine-setup",
			"machiner",
			"migration-fortress",
			"migration-inactive-flag",
			"migration-minion",
			"proxy-config-updater",
			"reboot-executor",
			"ssh-authkeys-updater",
			"ssh-session",
			"storage-provisioner",
			"termination-signal-handler",
			"trace",
			"trace-config-updater",
			"upgrade-agent-steps-runner",
			"upgrade-check-flag",
			"upgrade-check-gate",
			"upgrade-steps-flag",
			"upgrade-steps-gate",
			"upgrader",
			"valid-credential-flag",
		},
	)
}

func (s *ManifoldsSuite) TestManifoldNamesCAAS(c *tc.C) {
	s.assertManifoldNames(
		c,
		machine.CAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		[]string{
			"agent",
			"api-address-updater",
			"api-caller",
			"api-config-watcher",
			"clock",
			"controller-agent-config",
			"controller-agent-config-ready-flag",
			"controller-agent-config-ready-gate",
			"flight-recorder",
			"http-client",
			"log-router",
			"logging-config-updater",
			"loki-endpoint-updater",
			"migration-fortress",
			"migration-inactive-flag",
			"migration-minion",
			"proxy-config-updater",
			"termination-signal-handler",
			"trace",
			"trace-config-updater",
			"upgrade-agent-steps-runner",
			"upgrade-check-flag",
			"upgrade-check-gate",
			"upgrade-steps-flag",
			"upgrade-steps-gate",
			"upgrader",
			"valid-credential-flag",
		},
	)
}

func (*ManifoldsSuite) assertManifoldNames(c *tc.C, manifolds dependency.Manifolds, expectedKeys []string) {
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	c.Assert(keys, tc.SameContents, expectedKeys)
}

func (*ManifoldsSuite) TestFinalGraphRegistrationCounts(c *tc.C) {
	iaas := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})
	caas := machine.CAASManifolds(machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})

	// The task README targets a 40-registration IAAS graph, which includes the
	// machine-converter worker. Step 2 removed the machine-converter worker
	// entirely (see step-2 memory), so the final IAAS graph has 39
	// registrations. The CAAS graph matches the README's 27-registration
	// target.
	c.Check(len(iaas), tc.Equals, 39)
	c.Check(len(caas), tc.Equals, 27)
}

func (*ManifoldsSuite) TestManifoldsHaveClosedDependencies(c *tc.C) {
	for _, manifolds := range []dependency.Manifolds{
		machine.IAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		machine.CAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
	} {
		names := set.NewStrings()
		for name := range manifolds {
			names.Add(name)
		}
		for name, manifold := range manifolds {
			for _, input := range manifold.Inputs {
				c.Check(names.Contains(input), tc.IsTrue,
					tc.Commentf("manifold %q has unresolved input %q", name, input))
			}
		}
	}
}

func (*ManifoldsSuite) TestNoControllerRoleHousing(c *tc.C) {
	for _, manifolds := range []dependency.Manifolds{
		machine.IAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		machine.CAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
	} {
		for _, manifold := range manifolds {
			checkNotContains(c, manifold.Inputs, "is-controller-flag")
			checkNotContains(c, manifold.Inputs, "is-not-controller-flag")
			checkNotContains(c, manifold.Inputs, "is-primary-controller-flag")
			checkNotContains(c, manifold.Inputs, "state-config-watcher")
		}
	}
}

func (*ManifoldsSuite) TestUpgradesBlockMigration(c *tc.C) {
	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})
	manifold, ok := manifolds["migration-fortress"]
	c.Assert(ok, tc.IsTrue)

	checkContains(c, manifold.Inputs, "upgrade-check-flag")
	checkContains(c, manifold.Inputs, "upgrade-steps-flag")
}

func (s *ManifoldsSuite) TestMigrationGuardsUsed(c *tc.C) {
	exempt := set.NewStrings(
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"controller-agent-config",
		"controller-agent-config-ready-flag",
		"controller-agent-config-ready-gate",
		"deployer",
		"flight-recorder",
		"http-client",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"termination-signal-handler",
		"trace",
		"upgrade-agent-steps-runner",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrader",
		"valid-credential-flag",
	)
	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})
	for name, manifold := range manifolds {
		c.Logf("%s", name)
		if !exempt.Contains(name) {
			checkContains(c, manifold.Inputs, "migration-fortress")
			checkContains(c, manifold.Inputs, "migration-inactive-flag")
		}
	}
}

func (*ManifoldsSuite) TestAPICallerNonRecoverableErrorHandling(c *tc.C) {
	ag := &mockAgent{
		conf: mockConfig{
			dataPath: c.MkDir(),
		},
	}
	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent:           ag,
		PreUpgradeSteps: preUpgradeSteps,
	})

	c.Assert(manifolds["api-caller"], tc.Not(tc.IsNil))
	apiCaller := manifolds["api-caller"]

	// Check that when the api-caller maps non-recoverable errors to ErrTerminateAgent.
	err := apiCaller.Filter(apicaller.ErrConnectImpossible)
	c.Assert(err, tc.Equals, jworker.ErrTerminateAgent)
}

// removedManifoldsConfigFields lists the ManifoldsConfig fields that were
// deleted along with the controller graph and Dqlite build closure. The
// fields had no consumers in the machine graph; the test guards against
// them being reintroduced unnoticed.
var removedManifoldsConfigFields = []string{
	"AgentName", "StartupValueProvider", "ControllerUUID",
	"ControllerModelUUID", "ControllerAgentTag", "LogDir",
	"ControlSocketPath", "DataDir", "APIPort", "AgentPassword",
	"BootstrapLock", "ProxyReadyLock", "UpgradeDBLock",
	"NewDBWorkerFunc", "LocalLogSink", "ControllerLeaseDuration",
	"TransactionPruneInterval", "RegisterIntrospectionHTTPHandlers",
	"NewModelWorker", "MuxShutdownWait", "DependencyEngineMetrics",
	"NewEnvironFunc", "NewCAASBrokerFunc",
}

func (*ManifoldsSuite) TestManifoldsConfigHasNoControllerStartupPlumbing(c *tc.C) {
	reflectType := reflect.TypeFor[machine.ManifoldsConfig]()
	fields := set.NewStrings()
	for field := range reflectType.Fields() {
		fields.Add(field.Name)
	}
	for _, name := range removedManifoldsConfigFields {
		c.Check(fields.Contains(name), tc.IsFalse, tc.Commentf(
			"machine.ManifoldsConfig.%s was removed with the controller graph; do not reintroduce it", name))
	}
}

// jujuagentdSourceRoot returns the cmd/jujuagentd source directory that
// contains this test's package.
func jujuagentdSourceRoot(c *tc.C) string {
	_, file, _, ok := runtime.Caller(0)
	c.Assert(ok, tc.IsTrue)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// TestJujuagentdMachineSourceClosure proves the jujuagentd machine agent no
// longer references the Dqlite/controller startup plumbing removed with the
// controller graph. agent/model and agent/modeloperator still own model
// worker wiring for the unit agent, so the machine-only identifiers are only
// banned inside this package's source files.
func (*ManifoldsSuite) TestJujuagentdMachineSourceClosure(c *tc.C) {
	bannedAnywhere := []string{
		"machineControllerStartupValueProvider",
		"machineModelStartupValueProvider",
		"bootstrapStartupValues",
	}
	bannedImports := []string{
		`"github.com/juju/juju/internal/worker/dbaccessor"`,
		`"github.com/juju/juju/apiserver"`,
	}
	bannedInMachinePackage := []string{
		"StartupValueProvider",
		"NewDBWorkerFunc",
		"NewModelWorker",
		"DependencyEngineMetrics",
	}

	root := jujuagentdSourceRoot(c)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, banned := range bannedAnywhere {
			c.Check(strings.Contains(string(content), banned), tc.IsFalse, tc.Commentf(
				"%s references removed symbol %s", rel, banned))
		}
		for _, bannedImport := range bannedImports {
			c.Check(strings.Contains(string(content), bannedImport), tc.IsFalse, tc.Commentf(
				"%s imports removed dependency %s", rel, bannedImport))
		}
		if strings.HasPrefix(rel, filepath.Join("agent", "machine")) {
			for _, banned := range bannedInMachinePackage {
				c.Check(strings.Contains(string(content), banned), tc.IsFalse, tc.Commentf(
					"%s references removed machine-agent plumbing %s", rel, banned))
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func checkContains(c *tc.C, names []string, seek string) {
	if slices.Contains(names, seek) {
		return
	}
	c.Errorf("%q not found in %v", seek, names)
}

func checkNotContains(c *tc.C, names []string, seek string) {
	if slices.Contains(names, seek) {
		c.Errorf("%q found in %v", seek, names)
		return
	}
}

func (*ManifoldsSuite) TestUpgradeGates(c *tc.C) {
	upgradeStepsLock := gate.NewLock()
	upgradeCheckLock := gate.NewLock()
	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent:            &mockAgent{},
		PreUpgradeSteps:  preUpgradeSteps,
		UpgradeStepsLock: upgradeStepsLock,
		UpgradeCheckLock: upgradeCheckLock,
	})
	assertGate(c, manifolds["upgrade-steps-gate"], upgradeStepsLock)
	assertGate(c, manifolds["upgrade-check-gate"], upgradeCheckLock)
}

func assertGate(c *tc.C, manifold dependency.Manifold, unlocker gate.Unlocker) {
	w, err := manifold.Start(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	var waiter gate.Waiter
	err = manifold.Output(w, &waiter)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case <-waiter.Unlocked():
		c.Fatalf("expected gate to be locked")
	default:
	}

	unlocker.Unlock()

	select {
	case <-waiter.Unlocked():
	default:
		c.Fatalf("expected gate to be unlocked")
	}
}

type mockAgent struct {
	agent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() agent.Config {
	return &ma.conf
}

func (ma *mockAgent) ChangeConfig(f agent.ConfigMutator) error {
	return f(&ma.conf)
}

type mockConfig struct {
	agent.ConfigSetter
	tag      names.Tag
	ssiSet   bool
	ssi      controller.ControllerAgentInfo
	dataPath string
}

func (mc *mockConfig) Tag() names.Tag {
	if mc.tag == nil {
		return names.NewMachineTag("99")
	}
	return mc.tag
}

func (mc *mockConfig) Controller() names.ControllerTag {
	return testing.ControllerTag
}

func (mc *mockConfig) StateServingInfo() (controller.ControllerAgentInfo, bool) {
	return mc.ssi, mc.ssiSet
}

func (mc *mockConfig) ControllerAgentInfo() (controller.ControllerAgentInfo, bool) {
	return mc.ssi, mc.ssiSet
}

func (mc *mockConfig) APIInfo() (*api.Info, bool) {
	return &api.Info{
		Addrs:    []string{"0.1.2.3:1234"},
		CACert:   "mock-ca-cert",
		Tag:      mc.Tag(),
		Password: "mock-password",
	}, true
}

func (mc *mockConfig) SetStateServingInfo(info controller.ControllerAgentInfo) {
	mc.ssiSet = true
	mc.ssi = info
}

func (mc *mockConfig) LogDir() string {
	return "log-dir"
}

func (mc *mockConfig) DataDir() string {
	if mc.dataPath != "" {
		return mc.dataPath
	}
	return "data-dir"
}

func (mc *mockConfig) LoggingConfig() string {
	return ""
}

func preUpgradeSteps(model.ModelType) upgrades.PreUpgradeStepsFunc { return nil }
