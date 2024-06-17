// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"context"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	service "github.com/juju/juju/domain/machine/service"
	state "github.com/juju/juju/domain/machine/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatchWithCreate(c *gc.C) {
	machineService, watcherC := s.machineServiceAndWatcher(c)

	// Create a machine
	uuid, err := machineService.CreateMachine(context.Background(), "machine-1")
	c.Assert(err, gc.IsNil)

	// Assert the create change
	watcherC.AssertChange(uuid)
}

func (s *watcherSuite) TestWatchWithDelete(c *gc.C) {
	machineService, watcherC := s.machineServiceAndWatcher(c)

	// Create a machine
	uuid, err := machineService.CreateMachine(context.Background(), "machine-1")
	c.Assert(err, gc.IsNil)

	// Assert the first change
	watcherC.AssertChange(uuid)

	// Ensure that the changestream is idle.
	s.ModelSuite.AssertChangeStreamIdle(c)

	// Delete the machine
	err = machineService.DeleteMachine(context.Background(), "machine-1")
	c.Assert(err, gc.IsNil)

	// Assert the second change
	watcherC.AssertChange(uuid)
}

func (s *watcherSuite) machineServiceAndWatcher(c *gc.C) (*service.WatchableService, watchertest.StringsWatcherC) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "machine")
	machineService := service.NewWatchableService(
		state.NewState(
			func() (database.TxnRunner, error) { return factory() },
			loggertesting.WrapCheckLog(c),
		),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)

	watcher, err := machineService.WatchModelMachines(context.Background())
	c.Assert(err, gc.IsNil)
	watcherC := watchertest.NewStringsWatcherC(c, watcher)
	return machineService, watcherC
}
