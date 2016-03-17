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
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

var (
	alwaysModelWorkers = []string{
		"agent", "clock", "api-caller",
		"is-responsible-flag", "not-alive-flag", "not-dead-flag",
	}
	aliveModelWorkers = []string{
		"environ-tracker", "space-importer", "compute-provisioner",
		"storage-provisioner", "firewaller", "unit-assigner",
		"service-scaler", "instance-poller", "charm-revision-updater",
		"metric-worker", "state-cleaner", "status-history-pruner",
	}
	deadModelWorkers = []string{
		"environ-tracker", "undertaker",
	}
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
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			seen := set.NewStrings()
			for _, name := range names {
				err := getResource(name, nil)
				if errors.Cause(err) == dependency.ErrMissing {
					continue
				}
				if tracker.c.Check(err, jc.ErrorIsNil) {
					seen.Add(name)
				}
			}
			tracker.mu.Lock()
			defer tracker.mu.Unlock()
			tracker.current[uuid] = seen

			return nil, dependency.ErrMissing
		},
	}
}
