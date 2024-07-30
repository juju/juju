// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"context"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/agenttest"
	"github.com/juju/juju/cmd/jujud-controller/agent/machine"
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

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *ManifoldsSuite) TestStartFuncsIAAS(c *gc.C) {
	s.assertStartFuncs(c, machine.IAASManifolds(machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	}))
}

func (s *ManifoldsSuite) TestStartFuncsCAAS(c *gc.C) {
	s.assertStartFuncs(c, machine.CAASManifolds(machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
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
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		[]string{
			"agent-config-updater",
			"agent",
			"api-address-updater",
			"api-caller",
			"api-config-watcher",
			"api-server",
			"audit-config-updater",
			"bootstrap",
			"broker-tracker",

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
			"disk-manager",
			"external-controller-updater",
			"file-notify-watcher",
			"host-key-reporter",
			"http-server-args",
			"http-server",
			"provider-tracker",
			"instance-mutater",
			"is-bootstrap-flag",
			"is-bootstrap-gate",
			"is-controller-flag",
			"is-not-controller-flag",
			"is-primary-controller-flag",
			"lease-expiry",
			"lease-manager",
			"log-sender",
			"log-sink",
			"logging-config-updater",
			"lxd-container-provisioner",
			"machine-action-runner",
			"machine-setup",
			"machiner",
			"migration-fortress",
			"migration-inactive-flag",
			"migration-minion",
			"model-worker-manager",
			"object-store-s3-caller",
			"object-store",
			"peer-grouper",
			"presence",
			"provider-service-factory",
			"proxy-config-updater",
			"pubsub-forwarder",
			"query-logger",
			"reboot-executor",
			"s3-http-client",
			"secret-backend-rotate",
			"service-factory",
			"ssh-authkeys-updater",
			"ssh-identity-writer",
			"ssh-importer-http-client",
			"state-config-watcher",
			"state-converter",
			"state",
			"storage-provisioner",
			"termination-signal-handler",
			"tools-version-checker",
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
		},
	)
}

func (s *ManifoldsSuite) TestManifoldNamesCAAS(c *gc.C) {
	s.assertManifoldNames(c,
		machine.CAASManifolds(machine.ManifoldsConfig{
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		[]string{
			"agent-config-updater",
			"agent",
			"api-caller",
			"api-config-watcher",
			"api-server",
			"audit-config-updater",
			"bootstrap",

			"caas-units-manager",
			"central-hub",
			"certificate-watcher",
			"change-stream-pruner",
			"change-stream",
			"charmhub-http-client",
			"clock",
			"control-socket",
			"controller-agent-config",
			"db-accessor",
			"external-controller-updater",
			"file-notify-watcher",
			"http-server-args",
			"http-server",
			"provider-tracker",
			"is-bootstrap-flag",
			"is-bootstrap-gate",
			"is-controller-flag",
			"is-primary-controller-flag",
			"lease-expiry",
			"lease-manager",
			"log-sender",
			"log-sink",
			"logging-config-updater",
			"migration-fortress",
			"migration-inactive-flag",
			"migration-minion",
			"model-worker-manager",
			"object-store-s3-caller",
			"object-store",
			"peer-grouper",
			"presence",
			"provider-service-factory",
			"proxy-config-updater",
			"pubsub-forwarder",
			"query-logger",
			"s3-http-client",
			"secret-backend-rotate",
			"service-factory",
			"ssh-identity-writer",
			"ssh-importer-http-client",
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
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
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
		"file-notify-watcher",
		"global-clock-updater",
		"http-server-args",
		"http-server",
		"provider-tracker",
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
		"presence",
		"provider-service-factory",
		"pubsub-forwarder",
		"query-logger",
		"s3-http-client",
		"service-factory",
		"service-factory",
		"ssh-importer-http-client",
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
		c.Logf(name)
		if !exempt.Contains(name) {
			checkContains(c, manifold.Inputs, "migration-fortress")
			checkContains(c, manifold.Inputs, "migration-inactive-flag")
		}
	}
}

func (*ManifoldsSuite) TestSingularGuardsUsed(c *gc.C) {
	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})

	// Explicitly guarded by ifController.
	controllerWorkers := set.NewStrings(
		"certificate-watcher",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-primary-controller-flag",
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
		"audit-config-updater",
		"bootstrap",
		"control-socket",
		"http-server-args",
		"log-sink",
		"object-store",
		"object-store-s3-caller",
		"state",
	)

	// bootstrapWorkers are workers that are run directly run after bootstrap.
	bootstrapWorkers := set.NewStrings(
		"api-server",
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
		case bootstrapWorkers.Contains(name):
			checkContains(c, manifold.Inputs, "is-bootstrap-flag")
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
		Agent:           ag,
		PreUpgradeSteps: preUpgradeSteps,
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
		PreUpgradeSteps:  preUpgradeSteps,
		UpgradeStepsLock: upgradeStepsLock,
		UpgradeCheckLock: upgradeCheckLock,
	})
	assertGate(c, manifolds["upgrade-steps-gate"], upgradeStepsLock)
	assertGate(c, manifolds["upgrade-check-gate"], upgradeCheckLock)
}

func assertGate(c *gc.C, manifold dependency.Manifold, unlocker gate.Unlocker) {
	w, err := manifold.Start(context.Background(), nil)
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
			Agent:           &mockAgent{},
			PreUpgradeSteps: preUpgradeSteps,
		}),
		expectedMachineManifoldsWithDependenciesIAAS,
	)
}

func (s *ManifoldsSuite) TestManifoldsDependenciesCAAS(c *gc.C) {
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

	"agent-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"central-hub",
		"migration-fortress",
		"migration-inactive-flag",
		"state-config-watcher",
		"trace",
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
		"change-stream",
		"charmhub-http-client",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-server-args",
		"provider-tracker",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-s3-caller",
		"provider-service-factory",
		"query-logger",
		"s3-http-client",
		"service-factory",
		"ssh-importer-http-client",
		"state",
		"state-config-watcher",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-gate",
	},

	"audit-config-updater": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"bootstrap": {
		"agent",
		"change-stream",
		"charmhub-http-client",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-bootstrap-gate",
		"is-controller-flag",
		"lease-manager",
		"object-store",
		"object-store-s3-caller",
		"provider-service-factory",
		"query-logger",
		"s3-http-client",
		"service-factory",
		"state",
		"state-config-watcher",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
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
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
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
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

	"change-stream-pruner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"is-primary-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

	"charmhub-http-client": {},

	"clock": {},

	"control-socket": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"controller-agent-config": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"db-accessor": {
		"agent",
		"controller-agent-config",
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
		"change-stream",
		"charmhub-http-client",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-server-args",
		"provider-tracker",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-s3-caller",
		"provider-service-factory",
		"query-logger",
		"s3-http-client",
		"service-factory",
		"ssh-importer-http-client",
		"state",
		"state-config-watcher",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-gate",
	},

	"http-server-args": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"provider-tracker": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
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

	"is-bootstrap-flag": {
		"is-bootstrap-gate",
	},

	"is-bootstrap-gate": {},

	"is-controller-flag": {"agent", "state-config-watcher"},

	"is-not-controller-flag": {"agent", "state-config-watcher"},

	"is-primary-controller-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"is-controller-flag",
		"state-config-watcher",
	},

	"lease-expiry": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"is-primary-controller-flag",
		"query-logger",
		"state-config-watcher",
		"trace",
	},

	"lease-manager": {
		"agent",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
		"trace",
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

	"log-sink": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
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

	"model-worker-manager": {
		"agent",
		"certificate-watcher",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"log-sink",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"peer-grouper": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"presence": {"agent", "central-hub", "state-config-watcher"},

	"provider-service-factory": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
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

	"pubsub-forwarder": {
		"agent",
		"central-hub",
		"state-config-watcher",
	},

	"object-store": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"lease-manager",
		"object-store-s3-caller",
		"provider-service-factory",
		"query-logger",
		"s3-http-client",
		"service-factory",
		"state-config-watcher",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"object-store-s3-caller": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"s3-http-client",
		"service-factory",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
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

	"s3-http-client": {
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

	"service-factory": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"state-config-watcher",
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

	"ssh-importer-http-client": {},

	"state": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

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

	"termination-signal-handler": {},

	"trace": {
		"agent",
	},

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
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state-config-watcher",
		"upgrade-database-gate",
	},

	"upgrade-steps-flag": {"upgrade-steps-gate"},

	"upgrade-steps-gate": {},

	"upgrade-steps-runner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state-config-watcher",
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
		"trace",
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
		"change-stream",
		"charmhub-http-client",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-server-args",
		"provider-tracker",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-s3-caller",
		"provider-service-factory",
		"query-logger",
		"s3-http-client",
		"service-factory",
		"ssh-importer-http-client",
		"state",
		"state-config-watcher",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-gate",
	},

	"audit-config-updater": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"bootstrap": {
		"agent",
		"change-stream",
		"charmhub-http-client",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-bootstrap-gate",
		"is-controller-flag",
		"lease-manager",
		"object-store",
		"object-store-s3-caller",
		"provider-service-factory",
		"query-logger",
		"s3-http-client",
		"service-factory",
		"state",
		"state-config-watcher",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"central-hub": {"agent", "state-config-watcher"},

	"certificate-watcher": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"change-stream": {
		"agent",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

	"change-stream-pruner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"is-primary-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

	"charmhub-http-client": {},

	"clock": {},

	"control-socket": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"controller-agent-config": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"db-accessor": {
		"agent",
		"controller-agent-config",
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

	"trace": {
		"agent",
	},

	"http-server": {
		"agent",
		"api-server",
		"audit-config-updater",
		"central-hub",
		"certificate-watcher",
		"change-stream",
		"charmhub-http-client",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-server-args",
		"provider-tracker",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store",
		"object-store-s3-caller",
		"provider-service-factory",
		"query-logger",
		"s3-http-client",
		"ssh-importer-http-client",
		"service-factory",
		"state",
		"state-config-watcher",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-gate",
	},

	"http-server-args": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"provider-tracker": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"state-config-watcher",
	},

	"is-bootstrap-flag": {
		"is-bootstrap-gate",
	},

	"is-bootstrap-gate": {},

	"is-controller-flag": {"agent", "state-config-watcher"},

	"is-primary-controller-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"is-controller-flag",
		"state-config-watcher",
	},

	"lease-expiry": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"is-primary-controller-flag",
		"query-logger",
		"state-config-watcher",
		"trace",
	},

	"lease-manager": {
		"agent",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
		"trace",
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

	"log-sink": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
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

	"model-worker-manager": {
		"agent",
		"certificate-watcher",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"log-sink",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"peer-grouper": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"presence": {"agent", "central-hub", "state-config-watcher"},

	"provider-service-factory": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
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

	"pubsub-forwarder": {
		"agent",
		"central-hub",
		"state-config-watcher",
	},

	"object-store": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"lease-manager",
		"object-store-s3-caller",
		"provider-service-factory",
		"query-logger",
		"s3-http-client",
		"service-factory",
		"state-config-watcher",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"object-store-s3-caller": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"s3-http-client",
		"service-factory",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"query-logger": {
		"agent",
		"is-controller-flag",
		"state-config-watcher",
	},

	"s3-http-client": {
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

	"service-factory": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"state-config-watcher",
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

	"ssh-importer-http-client": {},

	"state": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"state-config-watcher": {"agent"},

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
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state-config-watcher",
		"upgrade-database-gate",
	},

	"upgrade-steps-flag": {"upgrade-steps-gate"},

	"upgrade-steps-gate": {},

	"upgrade-steps-runner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"provider-tracker",
		"is-controller-flag",
		"provider-service-factory",
		"query-logger",
		"service-factory",
		"state-config-watcher",
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

func preUpgradeSteps(state.ModelType) upgrades.PreUpgradeStepsFunc { return nil }
