// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"time"

	"github.com/juju/collections/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type modelSummaryWatcherInternalSuite struct{}

var _ = gc.Suite(&modelSummaryWatcherInternalSuite{})

// TestSummaryUpdateDropsLockBeforeSend ensures that a summary update blocked
// on the summaryChange channel is not holding w.mu. The watcher loop takes
// w.mu while initializing, before it receives from summaryChange, so holding
// the lock across the send deadlocks it.
func (s *modelSummaryWatcherInternalSuite) TestSummaryUpdateDropsLockBeforeSend(c *gc.C) {
	watcher := &modelSummaryWatcher{
		controller: &Controller{
			config: map[string]interface{}{
				"controller-name": "test-controller",
			},
		},
		summaryChange: make(chan ModelSummary),
		hashes:        make(map[string]string),
		visibleModels: set.NewStrings(),
	}

	entered := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		watcher.onSummaryUpdate("", modelSummaryPayload{
			summary: ModelSummary{UUID: "model-1-uuid"},
			visibleTo: func(string) bool {
				close(entered)
				return true
			},
			hash: "hash",
		})
	}()

	select {
	case <-entered:
	case <-time.After(testing.LongWait):
		c.Fatal("summary update did not start")
	}

	// The update is now between taking and releasing w.mu. Once it reaches
	// the blocked send the lock must be free.
	deadline := time.After(testing.LongWait)
	for {
		if watcher.mu.TryLock() {
			watcher.mu.Unlock()
			break
		}
		select {
		case <-deadline:
			c.Fatal("summary update held w.mu while sending")
		case <-time.After(time.Millisecond):
		}
	}

	select {
	case summary := <-watcher.summaryChange:
		c.Check(summary.UUID, gc.Equals, "model-1-uuid")
	case <-time.After(testing.LongWait):
		c.Fatal("summary update did not send")
	}

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatal("summary update did not finish")
	}
}
