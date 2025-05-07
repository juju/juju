// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	goyaml "gopkg.in/yaml.v2"

	coretesting "github.com/juju/juju/internal/testing"
)

// NewWorkerMatcher takes an EngineTracker, an engine manager id to
// monitor and the workers that are expected to be running and sets up
// a WorkerMatcher.
func NewWorkerMatcher(c *tc.C, tracker *EngineTracker, id string, workers []string) *WorkerMatcher {
	return &WorkerMatcher{
		c:       c,
		tracker: tracker,
		id:      id,
		expect:  set.NewStrings(workers...),
	}
}

// WorkerMatcher monitors the workers of a single engine manager,
// using an EngineTracker, for a given set of workers to be running.
type WorkerMatcher struct {
	c         *tc.C
	tracker   *EngineTracker
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
func WaitMatch(c *tc.C, match func() bool, maxWait time.Duration) {
	timeout := time.After(maxWait)
	for {
		if match() {
			return
		}
		select {
		case <-time.After(coretesting.ShortWait):
		case <-timeout:
			c.Fatalf("timed out waiting for workers")
		}
	}
}

// NewEngineTracker creates a type that can Install itself into a
// Manifolds map, and expose recent snapshots of running Workers.
func NewEngineTracker() *EngineTracker {
	return &EngineTracker{
		current: make(map[string]set.Strings),
		reports: make(map[string]map[string]interface{}),
	}
}

// EngineTracker tracks workers which have started.
type EngineTracker struct {
	mu      sync.Mutex
	current map[string]set.Strings
	reports map[string]map[string]interface{}
}

// Workers returns the most-recently-reported set of running workers.
func (tracker *EngineTracker) Workers(id string) set.Strings {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.current[id]
}

// Report returns the most-recently-reported self-report. It will
// only work if you hack up the relevant engine-starting code to
// include:
//
//	manifolds["self"] = dependency.SelfManifold(engine)
//
// or otherwise inject a suitable "self" manifold.
func (tracker *EngineTracker) Report(id string) map[string]interface{} {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.reports[id]
}

// Install injects a manifold named TEST-TRACKER into raw, which will
// depend on all other manifolds in raw and write currently-available
// worker information to the tracker (differentiating it from other
// tracked engines via the id param).
func (tracker *EngineTracker) Install(raw dependency.Manifolds, id string) error {
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

func (tracker *EngineTracker) startFunc(id string, names []string) dependency.StartFunc {
	return func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
		seen := set.NewStrings()
		for _, name := range names {
			err := getter.Get(name, nil)
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
		if err := getter.Get("self", &reporter); err == nil {
			report = reporter.Report()
		}

		select {
		case <-ctx.Done():
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
