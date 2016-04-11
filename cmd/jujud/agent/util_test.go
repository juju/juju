// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"sync"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/model"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

var (
	// These vars hold the per-model workers we expect to run in
	// various circumstances. Note the absence of dyingModelWorkers:
	// it's not a stable state, because it's responsible for making
	// the model Dead via the undertaker, so it can't be waited for
	// reliably.
	alwaysModelWorkers = []string{
		"agent",
		"api-caller",
		"api-config-watcher",
		"clock",
		"discover-spaces-check-gate",
		"is-responsible-flag",
		"not-alive-flag",
		"not-dead-flag",
	}
	aliveModelWorkers = []string{
		"charm-revision-updater",
		"compute-provisioner",
		"discover-spaces",
		"environ-tracker",
		"firewaller",
		"instance-poller",
		"metric-worker",
		"migration-master",
		"service-scaler",
		"state-cleaner",
		"status-history-pruner",
		"storage-provisioner",
		"unit-assigner",
	}
	deadModelWorkers = []string{
		"environ-tracker", "undertaker",
	}

	// ReallyLongTimeout should be long enough for the model-tracker
	// tests that depend on a hosted model; its backing state is not
	// accessible for StartSyncs, so we generally have to wait for at
	// least two 5s ticks to pass, and should expect rare circumstances
	// to take even longer.
	ReallyLongWait = coretesting.LongWait * 3
)

// modelMatchFunc returns a func that will return whether the current
// set of workers running for the supplied model matches those supplied;
// and will log what it saw in some detail.
func modelMatchFunc(c *gc.C, tracker *modelTracker, workers []string) func(string) bool {
	expect := set.NewStrings(workers...)
	return func(uuid string) bool {
		actual := tracker.Workers(uuid)
		c.Logf("\n%s: has workers %v", uuid, actual.SortedValues())
		extras := actual.Difference(expect)
		missed := expect.Difference(actual)
		if len(extras) == 0 && len(missed) == 0 {
			return true
		}
		c.Logf("%s: waiting for %v", uuid, missed.SortedValues())
		c.Logf("%s: unexpected %v", uuid, extras.SortedValues())
		return false
	}
}

// newModelTracker creates a type whose Manifolds method can
// be patched over modelManifolds, and whose Workers method
// will tell you what workers are currently running stably
// within the requested model's dependency engine.
func newModelTracker(c *gc.C) *modelTracker {
	return &modelTracker{
		c:       c,
		current: make(map[string]set.Strings),
	}
}

type modelTracker struct {
	c       *gc.C
	mu      sync.Mutex
	current map[string]set.Strings
}

func (tracker *modelTracker) Workers(model string) set.Strings {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.current[model]
}

func (tracker *modelTracker) Manifolds(config model.ManifoldsConfig) dependency.Manifolds {
	const trackerName = "TEST-TRACKER"
	raw := model.Manifolds(config)
	uuid := config.Agent.CurrentConfig().Model().Id()

	names := make([]string, 0, len(raw))
	for name := range raw {
		if name == trackerName {
			tracker.c.Errorf("manifold tracker used repeatedly")
			return raw
		} else {
			names = append(names, name)
		}
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if _, exists := tracker.current[uuid]; exists {
		tracker.c.Errorf("model %s started repeatedly", uuid)
		return raw
	}

	raw[trackerName] = tracker.manifold(uuid, names)
	return raw
}

func (tracker *modelTracker) manifold(uuid string, names []string) dependency.Manifold {
	return dependency.Manifold{
		Inputs: names,
		Start: func(context dependency.Context) (worker.Worker, error) {
			seen := set.NewStrings()
			for _, name := range names {
				err := context.Get(name, nil)
				if errors.Cause(err) == dependency.ErrMissing {
					continue
				}
				if tracker.c.Check(err, jc.ErrorIsNil) {
					seen.Add(name)
				}
			}
			select {
			case <-context.Abort():
				// don't bother to report if it's about to change
			default:
				tracker.mu.Lock()
				defer tracker.mu.Unlock()
				tracker.current[uuid] = seen
			}
			return nil, dependency.ErrMissing
		},
	}
}
