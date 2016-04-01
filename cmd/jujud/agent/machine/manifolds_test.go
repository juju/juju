// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"sort"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/gate"
)

type ManifoldsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ManifoldsSuite{})

func (*ManifoldsSuite) TestStartFuncs(c *gc.C) {
	manifolds := machine.Manifolds(machine.ManifoldsConfig{
		Agent: fakeAgent{},
	})
	for name, manifold := range manifolds {
		c.Logf("checking %q manifold", name)
		c.Check(manifold.Start, gc.NotNil)
	}
}

func (*ManifoldsSuite) TestManifoldNames(c *gc.C) {
	manifolds := machine.Manifolds(machine.ManifoldsConfig{})
	keys := make([]string, 0, len(manifolds))
	for k := range manifolds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	expectedKeys := []string{
		"agent",
		"api-address-updater",
		"api-caller",
		"apiworkers",
		"authenticationworker",
		"deployer",
		"disk-manager",
		"identity-file-writer",
		"log-sender",
		"logging-config-updater",
		"machiner",
		"proxy-config-updater",
		"reboot",
		"resumer",
		"serving-info-setter",
		"state",
		"state-config-watcher",
		"stateworkers",
		"storage-provisioner-machine",
		"termination",
		"tools-version-checker",
		"upgrade-check-gate",
		"upgrade-check-flag",
		"upgrade-steps-gate",
		"upgrade-steps-flag",
		"upgrader",
		"upgradesteps",
	}
	c.Assert(keys, jc.SameContents, expectedKeys)
}

func (*ManifoldsSuite) TestUpgradeGuardsUsed(c *gc.C) {
	exempt := set.NewStrings(
		"agent",
		"api-caller",
		"state",
		"state-config-watcher",
		"stateworkers",
		"termination",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrader",
		"upgradesteps",
	)
	manifolds := machine.Manifolds(machine.ManifoldsConfig{})
	keys := make([]string, 0, len(manifolds))
	for key := range manifolds {
		if !exempt.Contains(key) {
			keys = append(keys, key)
		}
	}
	for _, key := range keys {
		c.Logf("checking %s...", key)
		var sawCheck, sawSteps bool
		for _, name := range manifolds[key].Inputs {
			if name == "upgrade-check-flag" {
				sawCheck = true
			}
			if name == "upgrade-steps-flag" {
				sawSteps = true
			}
		}
		c.Check(sawSteps, jc.IsTrue)
		c.Check(sawCheck, jc.IsTrue)
	}
}

func (*ManifoldsSuite) TestUpgradeGates(c *gc.C) {
	upgradeStepsLock := gate.NewLock()
	upgradeCheckLock := gate.NewLock()
	manifolds := machine.Manifolds(machine.ManifoldsConfig{
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

type fakeAgent struct {
	agent.Agent
}
