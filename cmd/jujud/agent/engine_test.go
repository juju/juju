// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/machine"
	"github.com/juju/juju/cmd/jujud/agent/model"
	"github.com/juju/juju/cmd/jujud/agent/unit"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
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
		"not-alive-flag",
		"not-dead-flag",
		"spaces-imported-gate",
	}
	aliveModelWorkers = []string{
		"charm-revision-updater",
		"compute-provisioner",
		"environ-tracker",
		"firewaller",
		"instance-poller",
		"metric-worker",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-master",
		"application-scaler",
		"space-importer",
		"state-cleaner",
		"status-history-pruner",
		"storage-provisioner",
		"unit-assigner",
	}
	migratingModelWorkers = []string{
		"environ-tracker",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-master",
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
		"machine-lock",
		"migration-fortress",
		"migration-inactive-flag",
		"migration-minion",
		"upgrader",
	}
	notMigratingUnitWorkers = []string{
		"api-address-updater",
		"charm-dir",
		"hook-retry-strategy",
		"leadership-tracker",
		"logging-config-updater",
		"meter-status",
		"proxy-config-updater",
		"uniter",
	}

	alwaysMachineWorkers = []string{
		"agent",
		"api-caller",
		"api-config-watcher",
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
	}
	notMigratingMachineWorkers = []string{
		"api-address-updater",
		"disk-manager",
		// "host-key-reporter", not stable, exits when done
		"log-sender",
		"logging-config-updater",
		"machine-action-runner",
		"machiner",
		"proxy-config-updater",
		// "reboot-executor", not stable, fails due to lp:1588186
		"ssh-authkeys-updater",
		"storage-provisioner",
		"unconverted-api-workers",
		"unit-agent-deployer",
	}
)

type ModelManifoldsFunc func(config model.ManifoldsConfig) dependency.Manifolds

func TrackModels(c *gc.C, tracker *engineTracker, inner ModelManifoldsFunc) ModelManifoldsFunc {
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

func TrackMachines(c *gc.C, tracker *engineTracker, inner MachineManifoldsFunc) MachineManifoldsFunc {
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

func TrackUnits(c *gc.C, tracker *engineTracker, inner UnitManifoldsFunc) UnitManifoldsFunc {
	return func(config unit.ManifoldsConfig) dependency.Manifolds {
		raw := inner(config)
		id := config.Agent.CurrentConfig().Tag().String()
		if err := tracker.Install(raw, id); err != nil {
			c.Errorf("cannot install tracker: %v", err)
		}
		return raw
	}
}

// EngineMatchFunc returns a func that accepts an identifier for a
// single engine manager by the tracker; that will return true if the
// workers running in that engine match the supplied workers.
func EngineMatchFunc(c *gc.C, tracker *engineTracker, workers []string) func(string) bool {
	expect := set.NewStrings(workers...)
	return func(id string) bool {
		actual := tracker.Workers(id)
		c.Logf("\n%s: has workers %v", id, actual.SortedValues())
		extras := actual.Difference(expect)
		missed := expect.Difference(actual)
		if len(extras) == 0 && len(missed) == 0 {
			return true
		}
		c.Logf("%s: waiting for %v", id, missed.SortedValues())
		c.Logf("%s: unexpected %v", id, extras.SortedValues())
		return false
	}
}

// WaitMatch returns only when the match func succeeds, or it times out.
func WaitMatch(c *gc.C, match func(string) bool, id string, sync func()) {
	timeout := time.After(coretesting.LongWait)
	for {
		if match(id) {
			return
		}
		select {
		case <-time.After(coretesting.ShortWait):
			sync()
		case <-timeout:
			c.Fatalf("timed out waiting for workers")
		}
	}
}

// NewEngineTracker creates a type that can Install itself into a
// Manifolds map, and expose recent snapshots of running Workers.
func NewEngineTracker() *engineTracker {
	return &engineTracker{
		current: make(map[string]set.Strings),
	}
}

type engineTracker struct {
	mu      sync.Mutex
	current map[string]set.Strings
}

func (tracker *engineTracker) Workers(id string) set.Strings {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.current[id]
}

func (tracker *engineTracker) Install(raw dependency.Manifolds, id string) error {
	const trackerName = "TEST-TRACKER"
	names := make([]string, 0, len(raw))
	for name := range raw {
		if name == trackerName {
			return errors.New("engine tracker already installed")
		}
		names = append(names, name)
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if _, exists := tracker.current[id]; exists {
		return errors.Errorf("engine for %s already started", id)
	}
	raw[trackerName] = dependency.Manifold{
		Inputs: names,
		Start:  tracker.startFunc(id, names),
	}
	return nil
}

func (tracker *engineTracker) startFunc(id string, names []string) dependency.StartFunc {
	return func(context dependency.Context) (worker.Worker, error) {
		seen := set.NewStrings()
		for _, name := range names {
			err := context.Get(name, nil)
			switch errors.Cause(err) {
			case nil:
			case dependency.ErrMissing:
				continue
			default:
				name = fmt.Sprintf("%s [%v]", name, err)
			}
			seen.Add(name)
		}

		select {
		case <-context.Abort():
			// don't bother to report if it's about to change
		default:
			tracker.mu.Lock()
			defer tracker.mu.Unlock()
			tracker.current[id] = seen
		}
		return nil, dependency.ErrMissing
	}
}
