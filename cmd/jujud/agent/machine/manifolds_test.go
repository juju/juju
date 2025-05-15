// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/upgrades"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/state"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&ManifoldsSuite{})

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

func (*ManifoldsSuite) assertStartFuncs(c *tc.C, manifolds dependency.Manifolds) {
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, tc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNamesIAAS(c *tc.C) {
	s.assertManifoldNames(c,
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
			"charmhub-http-client",
			"clock",
			"deployer",
			"disk-manager",
			"host-key-reporter",
			"instance-mutater",
			"log-sender",
			"logging-config-updater",
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
			"ssh-identity-writer",
			"state-converter",
			"storage-provisioner",
			"termination-signal-handler",
			"tools-version-checker",
			"trace",
			"upgrade-check-flag",
			"upgrade-check-gate",
			"upgrade-steps-flag",
			"upgrade-steps-gate",
			"upgrade-steps-runner",
			"upgrader",
			"valid-credential-flag",
		},
	)
}

func (s *ManifoldsSuite) TestManifoldNamesCAAS(c *tc.C) {
	s.assertManifoldNames(c,
		machine.CAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		[]string{
			"agent",
			"api-caller",
			"api-config-watcher",
			"charmhub-http-client",
			"clock",
			"log-sender",
			"logging-config-updater",
			"migration-fortress",
			"migration-inactive-flag",
			"migration-minion",
			"proxy-config-updater",
			"ssh-identity-writer",
			"termination-signal-handler",
			"trace",
			"upgrade-check-flag",
			"upgrade-check-gate",
			"upgrade-steps-flag",
			"upgrade-steps-gate",
			"upgrade-steps-runner",
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
		"api-server",
		"audit-config-updater",
		"bootstrap",
		"central-hub",
		"certificate-updater",
		"certificate-watcher",
		"change-stream-pruner",
		"change-stream",
		"charmhub-http-client",
		"clock",
		"control-socket",
		"controller-agent-config",
		"db-accessor",
		"deployer",
		"domain-services",
		"file-notify-watcher",
		"global-clock-updater",
		"http-server-args",
		"http-server",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"is-not-controller-flag",
		"is-primary-controller-flag",
		"lease-expiry",
		"lease-manager",
		"log-sink",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"model-worker-manager",
		"object-store-s3-caller",
		"object-store",
		"peer-grouper",
		"pubsub-forwarder",
		"query-logger",
		"s3-http-client",
		"state-config-watcher",
		"state",
		"termination-signal-handler",
		"trace",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-database-runner",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrade-steps-runner",
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

func (*ManifoldsSuite) TestSingularGuardsUsed(c *tc.C) {
	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})

	// Explicitly guarded by ifController.
	controllerWorkers := set.NewStrings(
		"audit-config-updater",
		"certificate-watcher",
		"change-stream",
		"change-stream",
		"control-socket",
		"controller-agent-config",
		"db-accessor",
		"db-accessor",
		"file-notify-watcher",
		"file-notify-watcher",
		"is-primary-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-s3-caller",
		"query-logger",
		"query-logger",
		"s3-http-client",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-database-runner",
	)

	// Explicitly guarded by ifPrimaryController.
	primaryControllerWorkers := set.NewStrings(
		"change-stream-pruner",
		"external-controller-updater",
		"lease-expiry",
		"secret-backend-rotate",
	)

	// Ensure that at least one worker is guarded by ifDatabaseUpgradeComplete
	// flag. If no worker is guarded then we know that workers are accessing
	// the database before it has been upgraded.
	dbUpgradedWorkers := set.NewStrings(
		"bootstrap",
	)

	// bootstrapWorkers are workers that are run directly run after bootstrap.
	bootstrapWorkers := set.NewStrings()

	for name, manifold := range manifolds {
		c.Logf("%s", name)
		switch {
		case controllerWorkers.Contains(name):
			checkContains(c, manifold.Inputs, "is-controller-flag")
			checkNotContains(c, manifold.Inputs, "is-primary-controller-flag")
		case primaryControllerWorkers.Contains(name):
			checkNotContains(c, manifold.Inputs, "is-controller-flag")
			checkContains(c, manifold.Inputs, "is-primary-controller-flag")
		case dbUpgradedWorkers.Contains(name):
			checkNotContains(c, manifold.Inputs, "is-controller-flag")
			checkNotContains(c, manifold.Inputs, "is-primary-controller-flag")
			checkContains(c, manifold.Inputs, "upgrade-database-flag")
		case bootstrapWorkers.Contains(name):
			checkContains(c, manifold.Inputs, "is-bootstrap-flag")
		default:
			checkNotContains(c, manifold.Inputs, "is-controller-flag")
			checkNotContains(c, manifold.Inputs, "is-primary-controller-flag")
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

func checkContains(c *tc.C, names []string, seek string) {
	for _, name := range names {
		if name == seek {
			return
		}
	}
	c.Errorf("%q not found in %v", seek, names)
}

func checkNotContains(c *tc.C, names []string, seek string) {
	for _, name := range names {
		if name == seek {
			c.Errorf("%q found in %v", seek, names)
			return
		}
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
	defer worker.Stop(w)

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

func (s *ManifoldsSuite) TestManifoldsDependenciesIAAS(c *tc.C) {
	agenttest.AssertManifoldsDependencies(c,
		machine.IAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		expectedMachineManifoldsWithDependenciesIAAS,
	)
}

func (s *ManifoldsSuite) TestManifoldsDependenciesCAAS(c *tc.C) {
	agenttest.AssertManifoldsDependencies(c,
		machine.CAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		expectedMachineManifoldsWithDependenciesCAAS,
	)
}

var expectedMachineManifoldsWithDependenciesIAAS = map[string][]string{

	"agent": {},

	"api-address-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"api-caller": {"agent", "api-config-watcher"},

	"api-config-watcher": {"agent"},

	"broker-tracker": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"charmhub-http-client": {},

	"clock": {},

	"deployer": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"disk-manager": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"host-key-reporter": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"trace": {
		"agent",
	},

	"instance-mutater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"broker-tracker",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"log-sender": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"logging-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"lxd-container-provisioner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"machine-action-runner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"machine-setup": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"machiner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"migration-fortress": {
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"migration-inactive-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
	},

	"migration-minion": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"proxy-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"reboot-executor": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"ssh-authkeys-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"ssh-identity-writer": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"state-converter": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"storage-provisioner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"valid-credential-flag",
	},

	"termination-signal-handler": {},

	"tools-version-checker": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"upgrade-check-flag": {"upgrade-check-gate"},

	"upgrade-check-gate": {},

	"upgrade-steps-flag": {"upgrade-steps-gate"},

	"upgrade-steps-gate": {},

	"upgrade-steps-runner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"upgrade-steps-gate",
	},

	"upgrader": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"upgrade-check-gate",
		"upgrade-steps-gate",
	},

	"valid-credential-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
	},
}

var expectedMachineManifoldsWithDependenciesCAAS = map[string][]string{

	"agent": {},

	"api-caller": {"agent", "api-config-watcher"},

	"api-config-watcher": {"agent"},

	"charmhub-http-client": {},

	"clock": {},

	"trace": {
		"agent",
	},

	"log-sender": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"logging-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"migration-fortress": {
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"migration-inactive-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
	},

	"migration-minion": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"proxy-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"ssh-identity-writer": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"termination-signal-handler": {},

	"upgrade-check-flag": {"upgrade-check-gate"},

	"upgrade-check-gate": {},

	"upgrade-steps-flag": {"upgrade-steps-gate"},

	"upgrade-steps-gate": {},

	"upgrade-steps-runner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"upgrade-steps-gate",
	},

	"upgrader": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"upgrade-check-gate",
		"upgrade-steps-gate",
	},

	"valid-credential-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
	},
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
	ssi      controller.StateServingInfo
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

func (mc *mockConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	return mc.ssi, mc.ssiSet
}

func (mc *mockConfig) SetStateServingInfo(info controller.StateServingInfo) {
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

func preUpgradeSteps(state.ModelType) upgrades.PreUpgradeStepsFunc { return nil }
