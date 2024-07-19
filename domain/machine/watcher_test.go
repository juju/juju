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
	life "github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/machine/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite

	svc *service.WatchableService
}

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "machine")
	s.svc = service.NewWatchableService(
		state.NewState(
			func() (database.TxnRunner, error) { return factory() },
			loggertesting.WrapCheckLog(c),
		),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)
}

func (s *watcherSuite) TestWatchModelMachines(c *gc.C) {
	_, err := s.svc.CreateMachine(context.Background(), "0")
	c.Assert(err, gc.IsNil)

	_, err = s.svc.CreateMachine(context.Background(), "0/lxd/0")
	c.Assert(err, gc.IsNil)

	s.AssertChangeStreamIdle(c)

	watcher, err := s.svc.WatchModelMachines()
	c.Assert(err, gc.IsNil)
	defer watchertest.CleanKill(c, watcher)

	watcherC := watchertest.NewStringsWatcherC(c, watcher)

	// The initial event should have the machine we created prior,
	// but not the container.
	watcherC.AssertChange("0")

	// A new machine triggers an emission.
	_, err = s.svc.CreateMachine(context.Background(), "1")
	c.Assert(err, gc.IsNil)
	watcherC.AssertChange("1")

	// An update triggers an emission.
	err = s.svc.SetMachineLife(context.Background(), "1", life.Dying)
	c.Assert(err, gc.IsNil)
	watcherC.AssertChange("1")

	// A deletion is ignored.
	err = s.svc.DeleteMachine(context.Background(), "1")
	c.Assert(err, gc.IsNil)

	// As is a container creation.
	_, err = s.svc.CreateMachine(context.Background(), "0/lxd/1")
	c.Assert(err, gc.IsNil)

	s.AssertChangeStreamIdle(c)
	watcherC.AssertNoChange()
}

func (s *watcherSuite) TestMachineCloudInstanceWatchWithSet(c *gc.C) {
	watcher, err := s.svc.WatchMachineCloudInstances(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	watcherC := watchertest.NewStringsWatcherC(c, watcher)
	// Initial event.
	watcherC.AssertOneChange()
	s.AssertChangeStreamIdle(c)

	// Create a machineUUID and set its cloud instance.
	machineUUID, err := s.svc.CreateMachine(context.Background(), "machine-1")
	c.Assert(err, gc.IsNil)
	hc := instance.HardwareCharacteristics{
		Mem:      uintptr(1024),
		RootDisk: uintptr(256),
		CpuCores: uintptr(4),
		CpuPower: uintptr(75),
	}
	err = s.svc.SetMachineCloudInstance(context.Background(), machineUUID, "42", hc)
	c.Assert(err, gc.IsNil)

	// Assert the change.
	watcherC.AssertChange(machineUUID)
}

func (s *watcherSuite) TestMachineCloudInstanceWatchWithDelete(c *gc.C) {
	watcher, err := s.svc.WatchMachineCloudInstances(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	watcherC := watchertest.NewStringsWatcherC(c, watcher)
	// Initial event.
	watcherC.AssertOneChange()
	s.AssertChangeStreamIdle(c)

	// Create a machineUUID and set its cloud instance.
	machineUUID, err := s.svc.CreateMachine(context.Background(), "machine-1")
	c.Assert(err, gc.IsNil)
	hc := instance.HardwareCharacteristics{
		Mem:      uintptr(1024),
		RootDisk: uintptr(256),
		CpuCores: uintptr(4),
		CpuPower: uintptr(75),
	}
	err = s.svc.SetMachineCloudInstance(context.Background(), machineUUID, "42", hc)
	c.Assert(err, gc.IsNil)
	// Delete the cloud instance.
	err = s.svc.DeleteMachineCloudInstance(context.Background(), machineUUID)
	c.Assert(err, gc.IsNil)

	// Assert the changes.
	watcherC.AssertChange(machineUUID)
}

func uintptr(u uint64) *uint64 {
	return &u
}
