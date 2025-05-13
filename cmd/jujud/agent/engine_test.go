// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/tc"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	coretesting "github.com/juju/juju/internal/testing"
)

var (
	// ReallyLongTimeout should be long enough for the model-tracker
	// tests that depend on a hosted model; its backing state is not
	// accessible for StartSyncs, so we generally have to wait for at
	// least two 5s ticks to pass, and should expect rare circumstances
	// to take even longer.
	ReallyLongWait = coretesting.LongWait * 3

	alwaysMachineWorkers = []string{
		"agent",
		"api-caller",
		"api-config-watcher",
		"broker-tracker",
		"charmhub-http-client",
		"clock",
		"instance-mutater",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"state-config-watcher",
		"termination-signal-handler",
		"trace",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrader",
		"valid-credential-flag",
	}
	notMigratingMachineWorkers = []string{
		"api-address-updater",
		"deployer",
		"disk-manager",
		"is-bootstrap-flag",
		"is-bootstrap-gate",
		"is-controller-flag",
		"is-not-controller-flag",
		"kvm-container-provisioner",
		"log-sender",
		"logging-config-updater",
		"lxd-container-provisioner",
		"machine-action-runner",
		"machiner",
		"proxy-config-updater",
		"reboot-executor",
		"ssh-authkeys-updater",
		"storage-provisioner",
		"upgrade-series",
	}
)

type MachineManifoldsFunc func(config machine.ManifoldsConfig) dependency.Manifolds

func TrackMachines(c *tc.C, tracker *agenttest.EngineTracker, inner MachineManifoldsFunc) MachineManifoldsFunc {
	return func(config machine.ManifoldsConfig) dependency.Manifolds {
		raw := inner(config)
		id := config.Agent.CurrentConfig().Tag().String()
		if err := tracker.Install(raw, id); err != nil {
			c.Errorf("cannot install tracker: %v", err)
		}
		return raw
	}
}
