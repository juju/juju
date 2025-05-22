// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"sort"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

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
			"agent-config-updater",
			"agent",
			"api-address-updater",
			"api-caller",
			"api-config-watcher",
			"api-remote-caller",
			"api-server",
			"audit-config-updater",
			"bootstrap",
			"broker-tracker",
			"central-hub",
			"certificate-updater",
			"certificate-watcher",
			"change-stream-pruner",
			"change-stream",
			"clock",
			"control-socket",
			"controller-agent-config",
			"db-accessor",
			"deployer",
			"disk-manager",
			"domain-services",
			"external-controller-updater",
			"file-notify-watcher",
			"host-key-reporter",
			"http-client",
			"http-server-args",
			"http-server",
			"instance-mutater",
			"is-bootstrap-flag",
			"is-bootstrap-gate",
			"is-controller-flag",
			"is-not-controller-flag",
			"is-primary-controller-flag",
			"jwt-parser",
			"lease-expiry",
			"lease-manager",
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
			"object-store-fortress",
			"object-store-facade",
			"object-store-drainer",
			"object-store-s3-caller",
			"object-store-services",
			"object-store",
			"peer-grouper",
			"provider-services",
			"provider-tracker",
			"proxy-config-updater",
			"query-logger",
			"reboot-executor",
			"secret-backend-rotate",
			"ssh-authkeys-updater",
			"ssh-identity-writer",
			"ssh-server",
			"state-config-watcher",
			"state-converter",
			"state",
			"storage-provisioner",
			"storage-registry",
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

func (s *ManifoldsSuite) TestManifoldNamesCAAS(c *tc.C) {
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
			"api-remote-caller",
			"api-server",
			"audit-config-updater",
			"bootstrap",
			"central-hub",
			"certificate-watcher",
			"change-stream-pruner",
			"change-stream",
			"clock",
			"control-socket",
			"controller-agent-config",
			"db-accessor",
			"domain-services",
			"external-controller-updater",
			"file-notify-watcher",
			"http-client",
			"http-server-args",
			"http-server",
			"is-bootstrap-flag",
			"is-bootstrap-gate",
			"is-controller-flag",
			"is-primary-controller-flag",
			"jwt-parser",
			"lease-expiry",
			"lease-manager",
			"log-sink",
			"logging-config-updater",
			"migration-fortress",
			"migration-inactive-flag",
			"migration-minion",
			"model-worker-manager",
			"object-store-fortress",
			"object-store-facade",
			"object-store-drainer",
			"object-store-s3-caller",
			"object-store-services",
			"object-store",
			"peer-grouper",
			"provider-services",
			"provider-tracker",
			"proxy-config-updater",
			"query-logger",
			"secret-backend-rotate",
			"ssh-identity-writer",
			"ssh-server",
			"state-config-watcher",
			"state",
			"storage-registry",
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
		"api-remote-caller",
		"api-server",
		"audit-config-updater",
		"bootstrap",
		"central-hub",
		"certificate-updater",
		"certificate-watcher",
		"change-stream-pruner",
		"change-stream",
		"clock",
		"control-socket",
		"controller-agent-config",
		"db-accessor",
		"deployer",
		"domain-services",
		"file-notify-watcher",
		"global-clock-updater",
		"http-client",
		"http-server-args",
		"http-server",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"is-not-controller-flag",
		"is-primary-controller-flag",
		"jwt-parser",
		"lease-expiry",
		"lease-manager",
		"log-sink",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"model-worker-manager",
		"object-store-fortress",
		"object-store-facade",
		"object-store-drainer",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"peer-grouper",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"ssh-server",
		"state-config-watcher",
		"state",
		"storage-registry",
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
		"api-remote-caller",
		"certificate-watcher",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-primary-controller-flag",
		"jwt-parser",
		"query-logger",
		"ssh-server",
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
		"log-sink",
		"object-store",
		"object-store-s3-caller",
		"state",
	)

	// bootstrapWorkers are workers that are run directly run after bootstrap.
	bootstrapWorkers := set.NewStrings(
		"http-server-args",
	)

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

func (*ManifoldsSuite) TestObjectStoreDoesNotUseDomainServices(c *tc.C) {
	// The object-store is a dependency of the domain-services, so we can't have
	// circular dependencies between the two. Ensuring that the dependencies is
	// a good way to check that the domain-services isn't a dependency of the
	// object-store.

	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})

	// Ensure that the object-store doesn't have a hard dependency on the
	// domain-services.

	manifold := manifolds["object-store"]
	checkNotContains(c, manifold.Inputs, "domain-services")

	// Also check that it doesn't have a transitive dependency on the
	// domain-services.

	dependencies := agenttest.ManifoldDependencies(manifolds, manifold)
	c.Check(dependencies.Contains("domain-services"), tc.IsFalse)
}

func (*ManifoldsSuite) TestProviderTrackerDoesNotUseDomainServices(c *tc.C) {
	// The provider-tracker is a dependency of the domain-services, so we can't
	// have circular dependencies between the two. Ensuring that the
	// dependencies is a good way to check that the domain-services isn't a
	// dependency of the provider-tracker.

	manifolds := machine.IAASManifolds(machine.ManifoldsConfig{
		Agent:           &mockAgent{},
		PreUpgradeSteps: preUpgradeSteps,
	})

	// Ensure that the provider-tracker doesn't have a hard dependency on the
	// domain-services.

	manifold := manifolds["provider-tracker"]
	checkNotContains(c, manifold.Inputs, "domain-services")

	// Also check that it doesn't have a transitive dependency on the
	// domain-services.

	dependencies := agenttest.ManifoldDependencies(manifolds, manifold)
	c.Check(dependencies.Contains("domain-services"), tc.IsFalse)
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

	"agent-config-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"migration-fortress",
		"migration-inactive-flag",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
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

	"api-remote-caller": {
		"agent",
		"central-hub",
		"is-controller-flag",
		"state-config-watcher",
	},

	"api-server": {
		"agent",
		"audit-config-updater",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"http-server-args",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"jwt-parser",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"state",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-gate",
	},

	"audit-config-updater": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"bootstrap": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-bootstrap-gate",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"state",
		"storage-registry",
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
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"state",
		"storage-registry",
		"trace",
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
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"is-primary-controller-flag",
		"lease-manager",
		"query-logger",
		"state-config-watcher",
		"trace",
	},

	"clock": {},

	"control-socket": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
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
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"is-primary-controller-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"query-logger",
		"state-config-watcher",
		"trace",
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

	"http-client": {},

	"http-server": {
		"agent",
		"api-server",
		"audit-config-updater",
		"central-hub",
		"certificate-watcher",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"http-server-args",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"jwt-parser",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"state",
		"storage-registry",
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
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"state",
		"storage-registry",
		"trace",
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
		"provider-services",
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
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"lease-manager",
		"query-logger",
		"state-config-watcher",
		"trace",
	},

	"jwt-parser": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"lease-expiry": {
		"agent",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"is-primary-controller-flag",
		"lease-manager",
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
		"is-controller-flag",
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
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
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
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"state",
		"storage-registry",
		"trace",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"provider-services": {
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

	"object-store-fortress": {},

	"object-store-facade": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"query-logger",
		"state-config-watcher",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"object-store-drainer": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"object-store-fortress",
		"object-store-services",
		"query-logger",
		"state-config-watcher",
	},

	"object-store": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"object-store-s3-caller",
		"object-store-services",
		"query-logger",
		"state-config-watcher",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"object-store-services": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

	"object-store-s3-caller": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"object-store-services",
		"query-logger",
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

	"secret-backend-rotate": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"is-primary-controller-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"query-logger",
		"state-config-watcher",
		"trace",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"domain-services": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
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
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"state": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
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

	"storage-registry": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
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
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"upgrade-steps-flag": {"upgrade-steps-gate"},

	"upgrade-steps-gate": {},

	"upgrade-steps-runner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
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
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"migration-fortress",
		"migration-inactive-flag",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"api-caller": {"agent", "api-config-watcher"},

	"api-config-watcher": {"agent"},

	"api-remote-caller": {
		"agent",
		"central-hub",
		"is-controller-flag",
		"state-config-watcher",
	},

	"api-server": {
		"agent",
		"audit-config-updater",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"http-server-args",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"jwt-parser",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"state",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-gate",
	},

	"audit-config-updater": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"bootstrap": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-bootstrap-gate",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"state",
		"storage-registry",
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
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"is-primary-controller-flag",
		"lease-manager",
		"query-logger",
		"state-config-watcher",
		"trace",
	},

	"clock": {},

	"control-socket": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
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
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"is-primary-controller-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"query-logger",
		"state-config-watcher",
		"trace",
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

	"http-client": {},

	"http-server": {
		"agent",
		"api-server",
		"audit-config-updater",
		"central-hub",
		"certificate-watcher",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"http-server-args",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"jwt-parser",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"state",
		"storage-registry",
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
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"state",
		"storage-registry",
		"trace",
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
		"provider-services",
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
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"lease-manager",
		"query-logger",
		"state-config-watcher",
		"trace",
	},

	"jwt-parser": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"lease-expiry": {
		"agent",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"is-primary-controller-flag",
		"lease-manager",
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

	"log-sink": {
		"agent",
		"is-controller-flag",
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
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
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
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"state",
		"storage-registry",
		"trace",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"provider-services": {
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

	"object-store-fortress": {},

	"object-store-facade": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"query-logger",
		"state-config-watcher",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"object-store-drainer": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"object-store-fortress",
		"object-store-services",
		"query-logger",
		"state-config-watcher",
	},

	"object-store": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"object-store-s3-caller",
		"object-store-services",
		"query-logger",
		"state-config-watcher",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"object-store-services": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"query-logger",
		"state-config-watcher",
	},

	"object-store-s3-caller": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"object-store-services",
		"query-logger",
		"state-config-watcher",
		"upgrade-database-flag",
		"upgrade-database-gate",
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
		"clock",
		"controller-agent-config",
		"db-accessor",
		"is-controller-flag",
		"is-primary-controller-flag",
		"lease-manager",
		"migration-fortress",
		"migration-inactive-flag",
		"query-logger",
		"state-config-watcher",
		"trace",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"domain-services": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
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
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"state": {
		"agent",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"state-config-watcher": {"agent"},

	"storage-registry": {
		"agent",
		"change-stream",
		"controller-agent-config",
		"db-accessor",
		"file-notify-watcher",
		"is-controller-flag",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
	},

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
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
	},

	"upgrade-steps-flag": {"upgrade-steps-gate"},

	"upgrade-steps-gate": {},

	"upgrade-steps-runner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"change-stream",
		"clock",
		"controller-agent-config",
		"db-accessor",
		"domain-services",
		"file-notify-watcher",
		"http-client",
		"is-controller-flag",
		"lease-manager",
		"log-sink",
		"object-store-facade",
		"object-store-fortress",
		"object-store-s3-caller",
		"object-store-services",
		"object-store",
		"provider-services",
		"provider-tracker",
		"query-logger",
		"state-config-watcher",
		"storage-registry",
		"trace",
		"upgrade-database-flag",
		"upgrade-database-gate",
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
