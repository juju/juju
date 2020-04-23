// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"sort"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/testing"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/gate"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (s *ManifoldsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (ms *ManifoldsSuite) TestStartFuncsIAAS(c *gc.C) {
	ms.assertStartFuncs(c, machine.IAASManifolds(machine.ManifoldsConfig{
		Agent: &mockAgent{},
	}))
}

func (ms *ManifoldsSuite) TestStartFuncsCAAS(c *gc.C) {
	ms.assertStartFuncs(c, machine.CAASManifolds(machine.ManifoldsConfig{
		Agent: &mockAgent{},
	}))
}

func (*ManifoldsSuite) assertStartFuncs(c *gc.C, manifolds dependency.Manifolds) {
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, gc.NotNil)
	}
}

func (ms *ManifoldsSuite) TestManifoldNamesIAAS(c *gc.C) {
	ms.assertManifoldNames(c,
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
			"clock",
			"controller-port",
			"disk-manager",
			"external-controller-updater",
			"fan-configurer",
			"global-clock-updater",
			"host-key-reporter",
			"http-server",
			"http-server-args",
			"instance-mutater",
			"is-controller-flag",
			"is-primary-controller-flag",
			"lease-clock-updater",
			"lease-manager",
			"legacy-leases-flag",
			"log-sender",
			"logging-config-updater",
			"machine-action-runner",
			"machiner",
			"mgo-txn-resumer",
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
			"raft",
			"raft-backstop",
			"raft-clusterer",
			"raft-forwarder",
			"raft-leader-flag",
			"raft-transport",
			"reboot-executor",
			"restore-watcher",
			"ssh-authkeys-updater",
			"ssh-identity-writer",
			"state",
			"state-config-watcher",
			"storage-provisioner",
			"termination-signal-handler",
			"tools-version-checker",
			"transaction-pruner",
			"unconverted-api-workers",
			"unit-agent-deployer",
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
		},
	)
}

func (ms *ManifoldsSuite) TestManifoldNamesCAAS(c *gc.C) {
	ms.assertManifoldNames(c,
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
			"central-hub",
			"certificate-watcher",
			"clock",
			"controller-port",
			"external-controller-updater",
			"http-server",
			"http-server-args",
			"is-controller-flag",
			"is-primary-controller-flag",
			"lease-clock-updater",
			"lease-manager",
			"log-sender",
			"logging-config-updater",
			"mgo-txn-resumer",
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
			"raft",
			"raft-backstop",
			"raft-clusterer",
			"raft-forwarder",
			"raft-leader-flag",
			"raft-transport",
			"restore-watcher",
			"ssh-identity-writer",
			"state",
			"state-config-watcher",
			"termination-signal-handler",
			"transaction-pruner",
			"unconverted-api-workers",
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

func (ms *ManifoldsSuite) TestMigrationGuardsUsed(c *gc.C) {
	exempt := set.NewStrings(
		"agent",
		"api-caller",
		"api-config-watcher",
		"api-server",
		"audit-config-updater",
		"certificate-updater",
		"certificate-watcher",
		"central-hub",
		"clock",
		"controller-port",
		"global-clock-updater",
		"http-server",
		"http-server-args",
		"is-controller-flag",
		"is-primary-controller-flag",
		"lease-clock-updater",
		"lease-manager",
		"legacy-leases-flag",
		"log-forwarder",
		"model-cache",
		"model-cache-initialized-flag",
		"model-cache-initialized-gate",
		"model-worker-manager",
		"multiwatcher",
		"peer-grouper",
		"presence",
		"pubsub-forwarder",
		"restore-watcher",
		"state",
		"state-config-watcher",
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
		"raft",
		"raft-backstop",
		"raft-clusterer",
		"raft-forwarder",
		"raft-leader-flag",
		"raft-transport",
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
	controllerWorkers := set.NewStrings(
		"certificate-watcher",
		"audit-config-updater",
		"is-primary-controller-flag",
		"model-cache",
		"model-cache-initialized-flag",
		"model-cache-initialized-gate",
		"multiwatcher",
		"lease-manager",
		"legacy-leases-flag",
		"raft-transport",
		"upgrade-database-flag",
		"upgrade-database-gate",
		"upgrade-database-runner",
	)
	primaryControllerWorkers := set.NewStrings(
		"external-controller-updater",
		"transaction-pruner",
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

func (ms *ManifoldsSuite) TestManifoldsDependencies(c *gc.C) {
	agenttest.AssertManifoldsDependencies(c,
		machine.IAASManifolds(machine.ManifoldsConfig{
			Agent: &mockAgent{},
		}),
		expectedMachineManifoldsWithDependencies,
	)
}

var expectedMachineManifoldsWithDependencies = map[string][]string{

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
		"clock",
		"controller-port",
		"http-server-args",
		"is-controller-flag",
		"lease-manager",
		"model-cache",
		"model-cache-initialized-flag",
		"model-cache-initialized-gate",
		"multiwatcher",
		"raft-transport",
		"restore-watcher",
		"state",
		"state-config-watcher",
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
		"state",
		"state-config-watcher",
	},

	"clock": {},

	"controller-port": {
		"agent",
		"central-hub",
		"state",
		"state-config-watcher",
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

	"external-controller-updater": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"is-controller-flag",
		"is-primary-controller-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"state",
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

	"global-clock-updater": {
		"agent",
		"is-controller-flag",
		"legacy-leases-flag",
		"state",
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
		"clock",
		"controller-port",
		"http-server-args",
		"is-controller-flag",
		"lease-manager",
		"model-cache",
		"model-cache-initialized-flag",
		"model-cache-initialized-gate",
		"multiwatcher",
		"raft-transport",
		"restore-watcher",
		"state",
		"state-config-watcher",
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

	"is-controller-flag": {"agent", "state", "state-config-watcher"},

	"is-primary-controller-flag": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"is-controller-flag",
		"state",
		"state-config-watcher",
	},

	"lease-clock-updater": {
		"agent",
		"central-hub",
		"clock",
		"controller-port",
		"http-server-args",
		"is-controller-flag",
		"lease-manager",
		"raft",
		"raft-forwarder",
		"raft-leader-flag",
		"raft-transport",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"lease-manager": {
		"agent",
		"central-hub",
		"clock",
		"is-controller-flag",
		"state",
		"state-config-watcher",
	},

	"legacy-leases-flag": {
		"agent",
		"is-controller-flag",
		"state",
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

	"mgo-txn-resumer": {
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
		"state",
		"state-config-watcher",
	},

	"model-cache-initialized-gate": {
		"agent",
		"is-controller-flag",
		"state",
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

	"raft": {
		"agent",
		"central-hub",
		"clock",
		"controller-port",
		"http-server-args",
		"is-controller-flag",
		"raft-transport",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"raft-backstop": {
		"agent",
		"central-hub",
		"clock",
		"controller-port",
		"http-server-args",
		"is-controller-flag",
		"raft",
		"raft-transport",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"raft-clusterer": {
		"agent",
		"central-hub",
		"clock",
		"controller-port",
		"http-server-args",
		"is-controller-flag",
		"raft",
		"raft-leader-flag",
		"raft-transport",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"raft-forwarder": {
		"agent",
		"central-hub",
		"clock",
		"controller-port",
		"http-server-args",
		"is-controller-flag",
		"raft",
		"raft-leader-flag",
		"raft-transport",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"raft-leader-flag": {
		"agent",
		"central-hub",
		"clock",
		"controller-port",
		"http-server-args",
		"is-controller-flag",
		"raft",
		"raft-transport",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"raft-transport": {
		"agent",
		"central-hub",
		"clock",
		"controller-port",
		"http-server-args",
		"is-controller-flag",
		"state",
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

	"restore-watcher": {"agent", "state", "state-config-watcher"},

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

	"state": {"agent", "state-config-watcher"},

	"state-config-watcher": {"agent"},

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

	"transaction-pruner": {
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"is-controller-flag",
		"is-primary-controller-flag",
		"migration-fortress",
		"migration-inactive-flag",
		"state",
		"state-config-watcher",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
	},

	"unconverted-api-workers": {
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

	"unit-agent-deployer": {
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
		"state",
		"state-config-watcher",
		"upgrade-database-gate",
	},

	"upgrade-database-gate": {
		"agent",
		"is-controller-flag",
		"state",
		"state-config-watcher",
	},

	"upgrade-database-runner": {
		"agent",
		"is-controller-flag",
		"state",
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
