// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/testing"
)

// ModelSuite is used to provide a sql.DB reference to tests.
// It is pre-populated with the model schema.
type ModelSuite struct {
	testing.ModelSuite

	watchableDB *TestWatchableDB
}

// SetUpTest is responsible for setting up a testing database suite initialised
// with the model schema.
func (s *ModelSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	s.watchableDB = NewTestWatchableDB(c, s.ModelUUID(), s.TxnRunner())
}

func (s *ModelSuite) TearDownTest(c *gc.C) {
	if s.watchableDB != nil {
		// We could use workertest.DirtyKill here, but some workers are already
		// dead when we get here and it causes unwanted logs. This just ensures
		// that we don't have any addition workers running.
		killAndWait(c, s.watchableDB)
	}

	s.ModelSuite.TearDownTest(c)
}

// GetWatchableDB allows the ModelSuite to be a WatchableDBGetter
func (s *ModelSuite) GetWatchableDB(namespace string) (changestream.WatchableDB, error) {
	return s.watchableDB, nil
}

// AssertChangeStreamIdle returns if and when the change stream is idle.
// This is useful to ensure that the change stream is not processing any
// events before running a test.
func (s *ModelSuite) AssertChangeStreamIdle(c *gc.C) {
	timeout := time.After(jujutesting.LongWait)
	for {
		select {
		case state := <-s.watchableDB.states:
			if state == stateIdle {
				return
			}
		case <-timeout:
			c.Fatalf("timed out waiting for idle state")
		}
	}
}
