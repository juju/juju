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
	goyaml "gopkg.in/yaml.v2"

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
		"machine-undertaker",
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
		"log-forwarder",
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
		"reboot-executor",
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

// NewWorkerManager takes an engineTracker, an engine manager id to
// monitor and the workers that are expected to be running and sets up
// a WorkerManager.
func NewWorkerMatcher(c *gc.C, tracker *engineTracker, id string, workers []string) *WorkerMatcher {
	return &WorkerMatcher{
		c:       c,
		tracker: tracker,
		id:      id,
		expect:  set.NewStrings(workers...),
	}
}

// WorkerMatcher monitors the workers of a single engine manager,
// using an engineTracker, for a given set of workers to be running.
type WorkerMatcher struct {
	c         *gc.C
	tracker   *engineTracker
	id        string
	expect    set.Strings
	matchTime time.Time
}

// Check returns true if the workers which are expected to be running
// (as specified in the call to NewWorkerMatcher) are running and have
// been running for a short period (i.e. some indication of stability).
func (m *WorkerMatcher) Check() bool {
	if m.checkOnce() {
		now := time.Now()
		if m.matchTime.IsZero() {
			m.matchTime = now
			return false
		}
		// Only return that the required workers have started if they
		// have been stable for a little while.
		return now.Sub(m.matchTime) >= time.Second
	}
	// Required workers not running, reset the timestamp.
	m.matchTime = time.Time{}
	return false
}

func (m *WorkerMatcher) checkOnce() bool {
	actual := m.tracker.Workers(m.id)
	m.c.Logf("\n%s: has workers %v", m.id, actual.SortedValues())
	extras := actual.Difference(m.expect)
	missed := m.expect.Difference(actual)
	if len(extras) == 0 && len(missed) == 0 {
		return true
	}
	m.c.Logf("%s: waiting for %v", m.id, missed.SortedValues())
	m.c.Logf("%s: unexpected %v", m.id, extras.SortedValues())
	report, _ := goyaml.Marshal(m.tracker.Report(m.id))
	m.c.Logf("%s: report: \n%s\n", m.id, report)
	return false
}

// WaitMatch returns only when the match func succeeds, or it times out.
func WaitMatch(c *gc.C, match func() bool, maxWait time.Duration, sync func()) {
	timeout := time.After(maxWait)
	for {
		if match() {
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
		reports: make(map[string]map[string]interface{}),
	}
}

type engineTracker struct {
	mu      sync.Mutex
	current map[string]set.Strings
	reports map[string]map[string]interface{}
}

// Workers returns the most-recently-reported set of running workers.
func (tracker *engineTracker) Workers(id string) set.Strings {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.current[id]
}

// Report returns the most-recently-reported self-report. It will
// only work if you hack up the relevant engine-starting code to
// include:
//
//    manifolds["self"] = dependency.SelfManifold(engine)
//
// or otherwise inject a suitable "self" manifold.
func (tracker *engineTracker) Report(id string) map[string]interface{} {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.reports[id]
}

// Install injects a manifold named TEST-TRACKER into raw, which will
// depend on all other manifolds in raw and write currently-available
// worker information to the tracker (differentiating it from other
// tracked engines via the id param).
func (tracker *engineTracker) Install(raw dependency.Manifolds, id string) error {
	const trackerName = "TEST-TRACKER"

	names := make([]string, 0, len(raw))
	for name := range raw {
		if name == trackerName {
			return errors.New("engine tracker installed repeatedly")
		}
		names = append(names, name)
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if _, exists := tracker.current[id]; exists {
		return errors.Errorf("manifolds for %s created repeatedly", id)
	}
	raw[trackerName] = dependency.Manifold{
		Inputs: append(names, "self"),
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

		var report map[string]interface{}
		var reporter dependency.Reporter
		if err := context.Get("self", &reporter); err == nil {
			report = reporter.Report()
		}

		select {
		case <-context.Abort():
			// don't bother to report if it's about to change
		default:
			tracker.mu.Lock()
			defer tracker.mu.Unlock()
			tracker.current[id] = seen
			tracker.reports[id] = report
		}
		return nil, dependency.ErrMissing
	}
}
