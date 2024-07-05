// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
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
	_, err := machineService.CreateMachine(context.Background(), "machine-1")
	c.Assert(err, gc.IsNil)

	// Assert the create change
	watcherC.AssertChange("machine-1")
}

func (s *watcherSuite) TestWatchWithDelete(c *gc.C) {
	machineService, watcherC := s.machineServiceAndWatcher(c)

	// Create a machine
	_, err := machineService.CreateMachine(context.Background(), "machine-1")
	c.Assert(err, gc.IsNil)

	// Assert the first change
	watcherC.AssertChange("machine-1")

	// Ensure that the changestream is idle.
	s.ModelSuite.AssertChangeStreamIdle(c)

	// Delete the machine
	err = machineService.DeleteMachine(context.Background(), "machine-1")
	c.Assert(err, gc.IsNil)

	// Assert the second change
	watcherC.AssertChange("machine-1")
}

func (s *watcherSuite) TestMachineCloudInstanceWatchWithSet(c *gc.C) {
	// Set-up watcher and test watcher.
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "machine_cloud_instance")
	svc := service.NewWatchableService(
		state.NewState(
			func() (database.TxnRunner, error) { return factory() },
			loggertesting.WrapCheckLog(c),
		),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)
	watcher, err := svc.WatchMachineCloudInstances(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	watcherC := watchertest.NewStringsWatcherC(c, watcher)
	// Initial event.
	watcherC.AssertOneChange()
	s.AssertChangeStreamIdle(c)

	// Create a machineUUID and set its cloud instance.
	machineUUID, err := svc.CreateMachine(context.Background(), "machine-1")
	c.Assert(err, gc.IsNil)
	hc := instance.HardwareCharacteristics{
		Mem:      uintptr(1024),
		RootDisk: uintptr(256),
		CpuCores: uintptr(4),
		CpuPower: uintptr(75),
	}
	err = svc.SetMachineCloudInstance(context.Background(), machineUUID, "42", hc)
	c.Assert(err, gc.IsNil)

	// Assert the change.
	watcherC.AssertChange(machineUUID)
}

func (s *watcherSuite) TestMachineCloudInstanceWatchWithDelete(c *gc.C) {
	// Set-up watcher and test watcher.
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "machine_cloud_instance")
	svc := service.NewWatchableService(
		state.NewState(
			func() (database.TxnRunner, error) { return factory() },
			loggertesting.WrapCheckLog(c),
		),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)
	watcher, err := svc.WatchMachineCloudInstances(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	watcherC := watchertest.NewStringsWatcherC(c, watcher)
	// Initial event.
	watcherC.AssertOneChange()
	s.AssertChangeStreamIdle(c)

	// Create a machineUUID and set its cloud instance.
	machineUUID, err := svc.CreateMachine(context.Background(), "machine-1")
	c.Assert(err, gc.IsNil)
	hc := instance.HardwareCharacteristics{
		Mem:      uintptr(1024),
		RootDisk: uintptr(256),
		CpuCores: uintptr(4),
		CpuPower: uintptr(75),
	}
	err = svc.SetMachineCloudInstance(context.Background(), machineUUID, "42", hc)
	c.Assert(err, gc.IsNil)
	// Delete the cloud instance.
	err = svc.DeleteMachineCloudInstance(context.Background(), machineUUID)
	c.Assert(err, gc.IsNil)

	// Assert the changes.
	watcherC.AssertChange(machineUUID)
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

	watcher, err := machineService.WatchMachines(context.Background())
	c.Assert(err, gc.IsNil)
	watcherC := watchertest.NewStringsWatcherC(c, watcher)
	// Initial event.
	watcherC.AssertOneChange()
	s.AssertChangeStreamIdle(c)

	return machineService, watcherC
}

func uintptr(u uint64) *uint64 {
	return &u
}
