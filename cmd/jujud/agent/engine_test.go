// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/cmd/jujud/agent/model"
	"github.com/juju/juju/cmd/jujud/agent/unit"
	coretesting "github.com/juju/juju/testing"
)

var (
	// These vars hold the per-model workers we expect to run in
	// various circumstances. Note the absence of worker lists for
	// dying/dead states, because those states are not stable: if
	// they're working correctly the engine will be shut down.
	alwaysModelWorkers = []string{
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"is-responsible-flag",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"not-alive-flag",
		"not-dead-flag",
		"valid-credential-flag",
	}
	requireValidCredentialModelWorkers = []string{
		"action-pruner",          // tertiary dependency: will be inactive because migration workers will be inactive
		"application-scaler",     // tertiary dependency: will be inactive because migration workers will be inactive
		"charm-revision-updater", // tertiary dependency: will be inactive because migration workers will be inactive
		"compute-provisioner",
		"firewaller",
		"instance-poller",
		"machine-undertaker",      // tertiary dependency: will be inactive because migration workers will be inactive
		"metric-worker",           // tertiary dependency: will be inactive because migration workers will be inactive
		"migration-fortress",      // secondary dependency: will be inactive because depends on model-upgrader
		"migration-inactive-flag", // secondary dependency: will be inactive because depends on model-upgrader
		"migration-master",        // secondary dependency: will be inactive because depends on model-upgrader
		"model-upgrader",
		"remote-relations",      // tertiary dependency: will be inactive because migration workers will be inactive
		"state-cleaner",         // tertiary dependency: will be inactive because migration workers will be inactive
		"status-history-pruner", // tertiary dependency: will be inactive because migration workers will be inactive
		"storage-provisioner",   // tertiary dependency: will be inactive because migration workers will be inactive
		"undertaker",
		"unit-assigner", // tertiary dependency: will be inactive because migration workers will be inactive
	}
	aliveModelWorkers = []string{
		"action-pruner",
		"charm-revision-updater",
		"compute-provisioner",
		"environ-tracker",
		"firewaller",
		"instance-poller",
		"machine-undertaker",
		"metric-worker",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-master",
		"application-scaler",
		"state-cleaner",
		"status-history-pruner",
		"storage-provisioner",
		"unit-assigner",
		"remote-relations",
		"log-forwarder",
	}
	migratingModelWorkers = []string{
		"environ-tracker",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-master",
		"model-upgrade-gate",
		"model-upgraded-flag",
		"log-forwarder",
	}
	// ReallyLongTimeout should be long enough for the model-tracker
	// tests that depend on a hosted model; its backing state is not
	// accessible for StartSyncs, so we generally have to wait for at
	// least two 5s ticks to pass, and should expect rare circumstances
	// to take even longer.
	ReallyLongWait = coretesting.LongWait * 3

	alwaysUnitWorkers = []string{
		"agent",
		"api-caller",
		"api-config-watcher",
		"log-sender",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"upgrader",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrade-check-gate",
		"upgrade-check-flag",
	}
	notMigratingUnitWorkers = []string{
		"api-address-updater",
		"charm-dir",
		"hook-retry-strategy",
		"leadership-tracker",
		"logging-config-updater",
		"meter-status",
		"metric-collect",
		"metric-sender",
		"metric-spool",
		"proxy-config-updater",
		"uniter",
	}

	alwaysMachineWorkers = []string{
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"state-config-watcher",
		"termination-signal-handler",
		"upgrade-check-flag",
		"upgrade-check-gate",
		"upgrade-steps-flag",
		"upgrade-steps-gate",
		"upgrader",
		"valid-credential-flag",
	}
	notMigratingMachineWorkers = []string{
		"api-address-updater",
		"disk-manager",
		"fan-configurer",
		// "host-key-reporter", not stable, exits when done
		"log-sender",
		"logging-config-updater",
		"machine-action-runner",
		"machiner",
		"proxy-config-updater",
		"reboot-executor",
		"ssh-authkeys-updater",
		"storage-provisioner",
		"upgrade-series",
		"unconverted-api-workers",
		"unit-agent-deployer",
	}
)

type ModelManifoldsFunc func(config model.ManifoldsConfig) dependency.Manifolds

func TrackModels(c *gc.C, tracker *agenttest.EngineTracker, inner ModelManifoldsFunc) ModelManifoldsFunc {
	return func(config model.ManifoldsConfig) dependency.Manifolds {
		raw := inner(config)
		id := config.Agent.CurrentConfig().Model().Id()
		if err := tracker.Install(raw, id); err != nil {
			c.Errorf("cannot install tracker: %v", err)
		}
		return raw
	}
}

type MachineManifoldsFunc func(config machine.ManifoldsConfig) dependency.Manifolds

func TrackMachines(c *gc.C, tracker *agenttest.EngineTracker, inner MachineManifoldsFunc) MachineManifoldsFunc {
	return func(config machine.ManifoldsConfig) dependency.Manifolds {
		raw := inner(config)
		id := config.Agent.CurrentConfig().Tag().String()
		if err := tracker.Install(raw, id); err != nil {
			c.Errorf("cannot install tracker: %v", err)
		}
		return raw
	}
}

type UnitManifoldsFunc func(config unit.ManifoldsConfig) dependency.Manifolds

func TrackUnits(c *gc.C, tracker *agenttest.EngineTracker, inner UnitManifoldsFunc) UnitManifoldsFunc {
	return func(config unit.ManifoldsConfig) dependency.Manifolds {
		raw := inner(config)
		id := config.Agent.CurrentConfig().Tag().String()
		if err := tracker.Install(raw, id); err != nil {
			c.Errorf("cannot install tracker: %v", err)
		}
		return raw
	}
}
