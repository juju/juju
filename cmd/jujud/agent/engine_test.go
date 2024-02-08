// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/worker/v3/dependency"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud-controller/agent/model"
	"github.com/juju/juju/cmd/jujud/agent/agenttest"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	coretesting "github.com/juju/juju/testing"
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
		"syslog",
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
		"deployer",
		"disk-manager",
		"fan-configurer",
		// "host-key-reporter", not stable, exits when done
		"log-sender",
		"logging-config-updater",
		"lxd-container-provisioner",
		"kvm-container-provisioner",
		"machine-action-runner",
		//"machine-setup", exits when done
		"machiner",
		"proxy-config-updater",
		"reboot-executor",
		"ssh-authkeys-updater",
		"storage-provisioner",
		"upgrade-series",
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
