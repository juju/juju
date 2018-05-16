// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"sort"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/gate"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (*ManifoldsSuite) TestStartFuncs(c *gc.C) {
	manifolds := machine.Manifolds(machine.ManifoldsConfig{
		Agent: &mockAgent{},
	})
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, gc.NotNil)
	}
}

func (*ManifoldsSuite) TestManifoldNames(c *gc.C) {
	manifolds := machine.Manifolds(machine.ManifoldsConfig{
		Agent: &mockAgent{},
	})
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	expectedKeys := []string{
		"agent",
		"api-address-updater",
		"api-caller",
		"api-config-watcher",
		"api-server",
		"audit-config-updater",
		"central-hub",
		"certificate-updater",
		"certificate-watcher",
		"clock",
		"disk-manager",
		"external-controller-updater",
		"fan-configurer",
		"global-clock-updater",
		"host-key-reporter",
		"http-server",
		"is-controller-flag",
		"is-primary-controller-flag",
		"log-pruner",
		"log-sender",
		"logging-config-updater",
		"machine-action-runner",
		"machiner",
		"mgo-txn-resumer",
		"migration-fortress",
		"migration-minion",
		"migration-inactive-flag",
		"model-worker-manager",
		"peer-grouper",
		"presence",
		"proxy-config-updater",
		"pubsub-forwarder",
		"raft",
		"raft-clusterer",
		"raft-enabled-flag",
		"raft-leader-flag",
		"raft-transport",
		"reboot-executor",
		"restore-watcher",
		"serving-info-setter",
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
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrade-steps-runner",
		"upgrader",
	}
	c.Assert(keys, jc.SameContents, expectedKeys)
}

func (*ManifoldsSuite) TestUpgradesBlockMigration(c *gc.C) {
	manifolds := machine.Manifolds(machine.ManifoldsConfig{
		Agent: &mockAgent{},
	})
	manifold, ok := manifolds["migration-fortress"]
	c.Assert(ok, jc.IsTrue)

	checkContains(c, manifold.Inputs, "upgrade-check-flag")
	checkContains(c, manifold.Inputs, "upgrade-steps-flag")
}

func (*ManifoldsSuite) TestMigrationGuardsUsed(c *gc.C) {
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
		"global-clock-updater",
		"http-server",
		"is-controller-flag",
		"is-primary-controller-flag",
		"log-forwarder",
		"model-worker-manager",
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
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrade-steps-runner",
		"upgrader",
		"raft",
		"raft-clusterer",
		"raft-enabled-flag",
		"raft-leader-flag",
		"raft-transport",
	)
	manifolds := machine.Manifolds(machine.ManifoldsConfig{
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
	manifolds := machine.Manifolds(machine.ManifoldsConfig{
		Agent: &mockAgent{},
	})
	controllerWorkers := set.NewStrings(
		"certificate-watcher",
		"audit-config-updater",
		"is-primary-controller-flag",
		"raft-enabled-flag",
	)
	primaryControllerWorkers := set.NewStrings(
		"external-controller-updater",
		"log-pruner",
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
	manifolds := machine.Manifolds(machine.ManifoldsConfig{
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
