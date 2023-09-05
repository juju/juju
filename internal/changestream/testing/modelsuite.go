// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/schema/testing"
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

	s.watchableDB = NewTestWatchableDB(c, coredatabase.ControllerNS, s.TxnRunner())
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
