// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

// ControllerSuite is used to provide a sql.DB reference to tests.
// It is pre-populated with the controller schema.
type ControllerSuite struct {
	testing.ControllerSuite

	watchableDB *TestWatchableDB
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the controller schema.
func (s *ControllerSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)

	s.watchableDB = NewTestWatchableDB(c, coredatabase.ControllerNS, s.TxnRunner())
	c.Cleanup(func() {
		// We could use workertest.DirtyKill here, but some workers are already
		// dead when we get here and it causes unwanted logs. This just ensures
		// that we don't have any addition workers running.
		if s.watchableDB != nil {
			s.watchableDB.Kill()
			_ = s.watchableDB.Wait()
			s.watchableDB = nil
		}
	})
}

// GetWatchableDB allows the ControllerSuite to be a WatchableDBGetter
func (s *ControllerSuite) GetWatchableDB(ctx context.Context, namespace string) (changestream.WatchableDB, error) {
	return s.watchableDB, nil
}

// AssertChangeStreamIdle returns if and when the change stream is idle.
// This is useful to ensure that the change stream is not processing any
// events before running a test.
func (w *ControllerSuite) AssertChangeStreamIdle(c *tc.C) {
	timeout := time.After(jujutesting.LongWait)
	for {
		select {
		case states := <-w.watchableDB.states:
			for _, state := range states {
				if state == stateIdle {
					return
				}
			}
		case <-timeout:
			c.Fatalf("timed out waiting for idle state")
		}
	}
}
