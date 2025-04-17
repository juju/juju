// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	worker "github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/controller"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/testing"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *ManifoldsSuite) TestStartFuncsIAAS(c *gc.C) {
	s.assertStartFuncs(c, machine.IAASManifolds(machine.ManifoldsConfig{
		Agent: &mockAgent{},
	}))
}

func (s *ManifoldsSuite) TestStartFuncsCAAS(c *gc.C) {
	s.assertStartFuncs(c, machine.CAASManifolds(machine.ManifoldsConfig{
		Agent: &mockAgent{},
	}))
}

func (*ManifoldsSuite) assertStartFuncs(c *gc.C, manifolds dependency.Manifolds) {
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, gc.NotNil)
	}
}

func (s *ManifoldsSuite) TestManifoldNamesIAAS(c *gc.C) {
	s.assertManifoldNames(c,
		machine.IAASManifolds(machine.ManifoldsConfig{
			Agent: &mockAgent{},
		}),
		[]string{
			"agent",
			"agent-config-updater",
			"api-address-updater",
			"api-caller",
			"api-config-watcher",
			"api-server",
			"audit-config-updater",
			"broker-tracker",
			"central-hub",
			"certificate-updater",
			"certificate-watcher",
			"change-stream",
			"charmhub-http-client",
			"clock",
			"control-socket",
			"controller-port",
			"db-accessor",
			"deployer",
			"disk-manager",
			"external-controller-updater",
			"fan-configurer",
			"file-notify-watcher",
			"host-key-reporter",
			"http-server",
			"http-server-args",
			"instance-mutater",
			"is-controller-flag",
			"is-not-controller-flag",
			"is-primary-controller-flag",
			"jwt-parser",
			"lease-expiry",
			"lease-manager",
			"log-sender",
			"logging-config-updater",
			"lxd-container-provisioner",
			"kvm-container-provisioner",
			"machine-action-runner",
			"machine-setup",
			"machiner",
			"migration-fortress",
			"migration-minion",
			"migration-inactive-flag",
			"model-cache",
			"model-cache-initialized-flag",
			"model-cache-initialized-gate",
			"model-worker-manager",
			"multiwatcher",
			"peer-grouper",
			"presence",
			"proxy-config-updater",
			"pubsub-forwarder",
			"query-logger",
			"reboot-executor",
			"secret-backend-rotate",
			"ssh-authkeys-updater",
			"ssh-identity-writer",
			"ssh-tunneler",
			"ssh-session",
			"state",
			"state-config-watcher",
			"state-converter",
			"storage-provisioner",
			"syslog",
			"termination-signal-handler",
			"tools-version-checker",
			"upgrade-check-flag",
			"upgrade-check-gate",
			"upgrade-database-flag",
			"upgrade-database-gate",
			"upgrade-database-runner",
			"upgrade-series",
			"upgrade-steps-flag",
			"upgrade-steps-gate",
			"upgrade-steps-runner",
			"upgrader",
			"valid-credential-flag",
			"ssh-server",
		},
	)
}

func (s *ManifoldsSuite) TestManifoldNamesCAAS(c *gc.C) {
	s.assertManifoldNames(c,
		machine.CAASManifolds(machine.ManifoldsConfig{
			Agent: &mockAgent{},
		}),
		[]string{
			"agent",
			"agent-config-updater",
			"api-caller",
			"api-config-watcher",
			"api-server",
			"audit-config-updater",
			"caas-units-manager",
			"central-hub",
			"certificate-watcher",
			"change-stream",
			"charmhub-http-client",
			"clock",
			"control-socket",
			"controller-port",
			"db-accessor",
			"external-controller-updater",
			"file-notify-watcher",
			"http-server",
			"http-server-args",
			"is-controller-flag",
			"is-primary-controller-flag",
			"jwt-parser",
			"lease-expiry",
			"lease-manager",
			"log-sender",
			"logging-config-updater",
			"migration-fortress",
			"migration-minion",
			"migration-inactive-flag",
			"model-cache",
			"model-cache-initialized-flag",
			"model-cache-initialized-gate",
			"model-worker-manager",
			"multiwatcher",
			"peer-grouper",
			"presence",
			"proxy-config-updater",
			"pubsub-forwarder",
			"query-logger",
			"secret-backend-rotate",
			"ssh-identity-writer",
			"ssh-server",
			"ssh-tunneler",
			"state",
			"state-config-watcher",
			"syslog",
			"termination-signal-handler",
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
		},
	)
}

func (*ManifoldsSuite) assertManifoldNames(c *gc.C, manifolds dependency.Manifolds, expectedKeys []string) {
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	c.Assert(keys, jc.SameContents, expectedKeys)
}

func (*ManifoldsSuite) TestUpgradesBlockMigration(c *gc.C) {
	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent: &mockAgent{},
	})
	manifold, ok := manifolds["migration-fortress"]
	c.Assert(ok, jc.IsTrue)

	checkContains(c, manifold.Inputs, "upgrade-check-flag")
	checkContains(c, manifold.Inputs, "upgrade-steps-flag")
}

func (s *ManifoldsSuite) TestMigrationGuardsUsed(c *gc.C) {
	exempt := set.NewStrings(
		"agent",
		"api-caller",
		"api-config-watcher",
		"api-server",
		"audit-config-updater",
		"certificate-updater",
		"certificate-watcher",
		"central-hub",
		"change-stream",
		"charmhub-http-client",
		"clock",
		"control-socket",
		"controller-port",
		"db-accessor",
		"deployer",
		"file-notify-watcher",
		"global-clock-updater",
		"http-server",
		"http-server-args",
		"is-controller-flag",
		"is-not-controller-flag",
		"is-primary-controller-flag",
		"jwt-parser",
		"lease-expiry",
		"lease-manager",
		"log-forwarder",
		"model-cache",
		"model-cache-initialized-flag",
		"model-cache-initialized-gate",
		"model-worker-manager",
		"multiwatcher",
		"peer-grouper",
		"presence",
		"pubsub-forwarder",
		"query-logger",
		"state",
		"state-config-watcher",
		"ssh-server",
		"ssh-tunneler",
		"syslog",
		"termination-signal-handler",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-database-runner",
		"upgrade-series",
		"upgrade-series-enabled",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrade-steps-runner",
		"upgrader",
		"valid-credential-flag",
	)
	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent: &mockAgent{},
	})
	for name, manifold := range manifolds {
		c.Logf(name)
		if !exempt.Contains(name) {
			checkContains(c, manifold.Inputs, "migration-fortress")
			checkContains(c, manifold.Inputs, "migration-inactive-flag")
		}
	}
}

func (*ManifoldsSuite) TestSingularGuardsUsed(c *gc.C) {
	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent: &mockAgent{},
	})

	// Explicitly guarded by ifController.
	controllerWorkers := set.NewStrings(
		"certificate-watcher",
		"audit-config-updater",
		"is-primary-controller-flag",
		"model-cache-initialized-flag",
		"model-cache-initialized-gate",
		"lease-expiry",
		"lease-manager",
		"jwt-parser",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-database-runner",
		"db-accessor",
		"query-logger",
		"change-stream",
		"file-notify-watcher",
		"control-socket",
		"ssh-server",
		"ssh-tunneler",
	)

	// Explicitly guarded by ifPrimaryController.
	primaryControllerWorkers := set.NewStrings(
		"external-controller-updater",
		"secret-backend-rotate",
	)

	// Guarded by ifDatabaseUpgradeComplete,
	// which implies running on a controller.
	dbUpgradedWorkers := set.NewStrings(
		"model-cache",
		"multiwatcher",
	)

	for name, manifold := range manifolds {
		c.Logf(name)
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
		default:
			checkNotContains(c, manifold.Inputs, "is-controller-flag")
			checkNotContains(c, manifold.Inputs, "is-primary-controller-flag")
		}
	}
}

func (*ManifoldsSuite) TestAPICallerNonRecoverableErrorHandling(c *gc.C) {
	ag := &mockAgent{
		conf: mockConfig{
			dataPath: c.MkDir(),
		},
	}
	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent: ag,
	})

	c.Assert(manifolds["api-caller"], gc.Not(gc.IsNil))
	apiCaller := manifolds["api-caller"]

	// Check that when the api-caller maps non-recoverable errors to ErrTerminateAgent.
	err := apiCaller.Filter(apicaller.ErrConnectImpossible)
	c.Assert(err, gc.Equals, jworker.ErrTerminateAgent)
}

func checkContains(c *gc.C, names []string, seek string) {
	for _, name := range names {
		if name == seek {
			return
		}
	}
	c.Errorf("%q not found in %v", seek, names)
}

func checkNotContains(c *gc.C, names []string, seek string) {
	for _, name := range names {
		if name == seek {
			c.Errorf("%q found in %v", seek, names)
			return
		}
	}
}

func (*ManifoldsSuite) TestUpgradeGates(c *gc.C) {
	upgradeStepsLock := gate.NewLock()
	upgradeCheckLock := gate.NewLock()
	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent:            &mockAgent{},
		UpgradeStepsLock: upgradeStepsLock,
		UpgradeCheckLock: upgradeCheckLock,
	})
	assertGate(c, manifolds["upgrade-steps-gate"], upgradeStepsLock)
	assertGate(c, manifolds["upgrade-check-gate"], upgradeCheckLock)
}

func assertGate(c *gc.C, manifold dependency.Manifold, unlocker gate.Unlocker) {
	w, err := manifold.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	defer worker.Stop(w)

	var waiter gate.Waiter
	err = manifold.Output(w, &waiter)
	c.Assert(err, jc.ErrorIsNil)

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

func (s *ManifoldsSuite) TestManifoldsDependenciesIAAS(c *gc.C) {
	agenttest.AssertManifoldsDependencies(c,
		machine.IAASManifolds(machine.ManifoldsConfig{
			Agent: &mockAgent{},
		}),
		expectedMachineManifoldsWithDependenciesIAAS,
	)
}

func (s *ManifoldsSuite) TestManifoldsDependenciesCAAS(c *gc.C) {
	agenttest.AssertManifoldsDependencies(c,
		machine.CAASManifolds(machine.ManifoldsConfig{
			Agent: &mockAgent{},
		}),
		expectedMachineManifoldsWithDependenciesCAAS,
	)
}

var expectedMachineManifoldsWithDependenciesIAAS = map[string][]string{

	"agent": {},

	"agent-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"central-hub",
		"migration-fortress",
		"migration-inactive-flag",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

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

	"api-server": {
		"agent",
		"audit-config-updater",
		"central-hub",
		"charmhub-http-client",
		"clock",
		"controller-port",
		"db-accessor",
		"http-server-args",
		"is-controller-flag",
		"jwt-parser",
		"lease-manager",
		"model-cache",
		"model-cache-initialized-flag",
		"model-cache-initialized-gate",
		"multiwatcher",
		"query-logger",
		"state",
		"state-config-watcher",
		"syslog",
		"upgrade-steps-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"audit-config-updater": {
		"agent",
		"is-controller-flag",
		"state",
		"state-config-watcher",
	},

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

	"central-hub": {"agent", "state-config-watcher"},

	"certificate-updater": {
		"agent",
		"certificate-watcher",
		"is-controller-flag",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"certificate-watcher": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"change-stream": {
		"agent",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

	"charmhub-http-client": {},

	"clock": {},

	"control-socket": {
		"agent",
		"is-controller-flag",
		"state",
		"state-config-watcher",
	},

	"controller-port": {
		"agent",
		"central-hub",
		"state",
		"state-config-watcher",
	},

	"db-accessor": {
		"agent",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

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

	"external-controller-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"is-controller-flag",
		"is-primary-controller-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"fan-configurer": {
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

	"file-notify-watcher": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
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

	"http-server": {
		"agent",
		"api-server",
		"audit-config-updater",
		"central-hub",
		"certificate-watcher",
		"charmhub-http-client",
		"clock",
		"controller-port",
		"db-accessor",
		"http-server-args",
		"is-controller-flag",
		"jwt-parser",
		"lease-manager",
		"model-cache",
		"model-cache-initialized-flag",
		"model-cache-initialized-gate",
		"multiwatcher",
		"query-logger",
		"state",
		"state-config-watcher",
		"syslog",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-gate",
	},

	"http-server-args": {
		"agent",
		"central-hub",
		"clock",
		"controller-port",
		"state",
		"state-config-watcher",
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

	"is-controller-flag": {"agent", "state-config-watcher"},

	"is-not-controller-flag": {"agent", "state-config-watcher"},

	"is-primary-controller-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"is-controller-flag",
		"state-config-watcher",
	},

	"jwt-parser": {
		"agent",
		"is-controller-flag",
		"state",
		"state-config-watcher",
	},

	"lease-expiry": {
		"agent",
		"clock",
		"db-accessor",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

	"lease-manager": {
		"agent",
		"clock",
		"db-accessor",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
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

	"kvm-container-provisioner": {
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
		"fan-configurer",
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

	"model-cache": {
		"agent",
		"central-hub",
		"is-controller-flag",
		"model-cache-initialized-gate",
		"multiwatcher",
		"state",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"model-cache-initialized-flag": {
		"agent",
		"is-controller-flag",
		"model-cache-initialized-gate",
		"state-config-watcher",
	},

	"model-cache-initialized-gate": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"model-worker-manager": {
		"agent",
		"central-hub",
		"certificate-watcher",
		"clock",
		"controller-port",
		"http-server-args",
		"is-controller-flag",
		"state",
		"state-config-watcher",
		"syslog",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"multiwatcher": {
		"agent",
		"is-controller-flag",
		"state",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"peer-grouper": {
		"agent",
		"central-hub",
		"clock",
		"controller-port",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"presence": {"agent", "central-hub", "state-config-watcher"},

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

	"pubsub-forwarder": {
		"agent",
		"central-hub",
		"state-config-watcher",
	},

	"query-logger": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
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

	"secret-backend-rotate": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"is-controller-flag",
		"is-primary-controller-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"state-config-watcher",
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

	"ssh-server": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"is-controller-flag",
		"jwt-parser",
		"state",
		"state-config-watcher",
		"ssh-tunneler",
	},

	"ssh-session": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"migration-fortress",
		"migration-inactive-flag",
		"ssh-authkeys-updater",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"ssh-tunneler": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"is-controller-flag",
		"state-config-watcher",
	},

	"state": {"agent", "state-config-watcher"},

	"state-config-watcher": {"agent"},

	"state-converter": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"is-not-controller-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"state-config-watcher",
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

	"syslog": {},

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

	"upgrade-database-flag": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
		"upgrade-database-gate",
	},

	"upgrade-database-gate": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"upgrade-database-runner": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
		"upgrade-database-gate",
	},

	"upgrade-series": {
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

	"agent-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"central-hub",
		"migration-fortress",
		"migration-inactive-flag",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"api-caller": {"agent", "api-config-watcher"},

	"api-config-watcher": {"agent"},

	"api-server": {
		"agent",
		"audit-config-updater",
		"central-hub",
		"charmhub-http-client",
		"clock",
		"controller-port",
		"db-accessor",
		"http-server-args",
		"is-controller-flag",
		"lease-manager",
		"jwt-parser",
		"model-cache",
		"model-cache-initialized-flag",
		"model-cache-initialized-gate",
		"multiwatcher",
		"query-logger",
		"state",
		"state-config-watcher",
		"syslog",
		"upgrade-steps-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"audit-config-updater": {
		"agent",
		"is-controller-flag",
		"state",
		"state-config-watcher",
	},

	"central-hub": {"agent", "state-config-watcher"},

	"certificate-watcher": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"change-stream": {
		"agent",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

	"charmhub-http-client": {},

	"clock": {},

	"control-socket": {
		"agent",
		"is-controller-flag",
		"state",
		"state-config-watcher",
	},

	"controller-port": {
		"agent",
		"central-hub",
		"state",
		"state-config-watcher",
	},

	"db-accessor": {
		"agent",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

	"external-controller-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"is-controller-flag",
		"is-primary-controller-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"file-notify-watcher": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"http-server": {
		"agent",
		"api-server",
		"audit-config-updater",
		"central-hub",
		"certificate-watcher",
		"charmhub-http-client",
		"clock",
		"controller-port",
		"db-accessor",
		"http-server-args",
		"is-controller-flag",
		"jwt-parser",
		"lease-manager",
		"model-cache",
		"model-cache-initialized-flag",
		"model-cache-initialized-gate",
		"multiwatcher",
		"query-logger",
		"state",
		"state-config-watcher",
		"syslog",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-gate",
	},

	"http-server-args": {
		"agent",
		"central-hub",
		"clock",
		"controller-port",
		"state",
		"state-config-watcher",
	},

	"is-controller-flag": {"agent", "state-config-watcher"},

	"is-primary-controller-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"is-controller-flag",
		"state-config-watcher",
	},

	"jwt-parser": {
		"agent",
		"is-controller-flag",
		"state",
		"state-config-watcher",
	},

	"lease-expiry": {
		"agent",
		"clock",
		"db-accessor",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

	"lease-manager": {
		"agent",
		"clock",
		"db-accessor",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
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

	"model-cache": {
		"agent",
		"central-hub",
		"is-controller-flag",
		"model-cache-initialized-gate",
		"multiwatcher",
		"state",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"model-cache-initialized-flag": {
		"agent",
		"is-controller-flag",
		"model-cache-initialized-gate",
		"state-config-watcher",
	},

	"model-cache-initialized-gate": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"model-worker-manager": {
		"agent",
		"central-hub",
		"certificate-watcher",
		"clock",
		"controller-port",
		"http-server-args",
		"is-controller-flag",
		"state",
		"state-config-watcher",
		"syslog",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"multiwatcher": {
		"agent",
		"is-controller-flag",
		"state",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"peer-grouper": {
		"agent",
		"central-hub",
		"clock",
		"controller-port",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"presence": {"agent", "central-hub", "state-config-watcher"},

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

	"pubsub-forwarder": {
		"agent",
		"central-hub",
		"state-config-watcher",
	},

	"query-logger": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"secret-backend-rotate": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"is-controller-flag",
		"is-primary-controller-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"state-config-watcher",
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

	"ssh-server": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"is-controller-flag",
		"jwt-parser",
		"state",
		"state-config-watcher",
		"ssh-tunneler",
	},

	"ssh-tunneler": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"is-controller-flag",
		"state-config-watcher",
	},

	"state": {"agent", "state-config-watcher"},

	"state-config-watcher": {"agent"},

	"syslog": {},

	"termination-signal-handler": {},

	"upgrade-check-flag": {"upgrade-check-gate"},

	"upgrade-check-gate": {},

	"upgrade-database-flag": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
		"upgrade-database-gate",
	},

	"upgrade-database-gate": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"upgrade-database-runner": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
		"upgrade-database-gate",
	},

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

	"caas-units-manager": {
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
