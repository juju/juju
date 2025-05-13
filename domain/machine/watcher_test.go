// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/machine/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite

	svc *service.WatchableService
}

var _ = tc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "machine")
	s.svc = service.NewWatchableService(
		state.NewState(
			func() (database.TxnRunner, error) { return factory() },
			clock.WallClock,
			loggertesting.WrapCheckLog(c),
		),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		nil,
	)
}

func (s *watcherSuite) TestWatchModelMachines(c *tc.C) {
	_, err := s.svc.CreateMachine(context.Background(), "0")
	c.Assert(err, tc.IsNil)

	_, err = s.svc.CreateMachine(context.Background(), "0/lxd/0")
	c.Assert(err, tc.IsNil)

	s.AssertChangeStreamIdle(c)

	watcher, err := s.svc.WatchModelMachines()
	c.Assert(err, tc.IsNil)
	defer watchertest.CleanKill(c, watcher)

	watcherC := watchertest.NewStringsWatcherC(c, watcher)

	// The initial event should have the machine we created prior,
	// but not the container.
	watcherC.AssertChange("0")

	// A new machine triggers an emission.
	_, err = s.svc.CreateMachine(context.Background(), "1")
	c.Assert(err, tc.IsNil)
	watcherC.AssertChange("1")

	// An update triggers an emission.
	err = s.svc.SetMachineLife(context.Background(), "1", life.Dying)
	c.Assert(err, tc.IsNil)
	watcherC.AssertChange("1")

	// A deletion is ignored.
	err = s.svc.DeleteMachine(context.Background(), "1")
	c.Assert(err, tc.IsNil)

	// As is a container creation.
	_, err = s.svc.CreateMachine(context.Background(), "0/lxd/1")
	c.Assert(err, tc.IsNil)

	s.AssertChangeStreamIdle(c)
	watcherC.AssertNoChange()
}

func (s *watcherSuite) TestMachineCloudInstanceWatchWithSet(c *tc.C) {
	// Create a machineUUID and set its cloud instance.
	machineUUID, err := s.svc.CreateMachine(context.Background(), "machine-1")
	c.Assert(err, tc.IsNil)
	hc := &instance.HardwareCharacteristics{
		Mem:      uintptr(1024),
		RootDisk: uintptr(256),
		CpuCores: uintptr(4),
		CpuPower: uintptr(75),
	}
	watcher, err := s.svc.WatchMachineCloudInstances(context.Background(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Should notify when the machine cloud instance is set.
	harness.AddTest(func(c *tc.C) {
		err = s.svc.SetMachineCloudInstance(context.Background(), machineUUID, "42", "", hc)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestMachineCloudInstanceWatchWithDelete(c *tc.C) {
	// Create a machineUUID and set its cloud instance.
	machineUUID, err := s.svc.CreateMachine(context.Background(), "machine-1")
	c.Assert(err, tc.IsNil)
	hc := &instance.HardwareCharacteristics{
		Mem:      uintptr(1024),
		RootDisk: uintptr(256),
		CpuCores: uintptr(4),
		CpuPower: uintptr(75),
	}
	err = s.svc.SetMachineCloudInstance(context.Background(), machineUUID, "42", "", hc)
	c.Assert(err, tc.IsNil)

	watcher, err := s.svc.WatchMachineCloudInstances(context.Background(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Should notify when the machine cloud instance is deleted.
	harness.AddTest(func(c *tc.C) {
		err = s.svc.DeleteMachineCloudInstance(context.Background(), machineUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchLXDProfiles(c *tc.C) {
	machineUUIDm0, err := s.svc.CreateMachine(context.Background(), "machine-1")
	c.Assert(err, tc.ErrorIsNil)
	err = s.svc.SetMachineCloudInstance(context.Background(), machineUUIDm0, instance.Id("123"), "", nil)
	c.Assert(err, tc.ErrorIsNil)
	machineUUIDm1, err := s.svc.CreateMachine(context.Background(), "machine-2")
	c.Assert(err, tc.ErrorIsNil)
	err = s.svc.SetMachineCloudInstance(context.Background(), machineUUIDm1, instance.Id("456"), "", nil)
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := s.svc.WatchLXDProfiles(context.Background(), machineUUIDm0)
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Should notify when a new profile is added.
	harness.AddTest(func(c *tc.C) {
		err := s.svc.SetAppliedLXDProfileNames(context.Background(), machineUUIDm0, []string{"profile-0"})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Should notify when profiles are overwritten.
	harness.AddTest(func(c *tc.C) {
		err := s.svc.SetAppliedLXDProfileNames(context.Background(), machineUUIDm0, []string{"profile-0", "profile-1", "profile-2"})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Nothing to notify when the lxd profiles are set on the other (non
	// watched) machine.
	harness.AddTest(func(c *tc.C) {
		err := s.svc.SetAppliedLXDProfileNames(context.Background(), machineUUIDm1, []string{"profile-0"})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

// TestWatchMachineForReboot tests the functionality of watching machines for reboot.
// It creates a machine hierarchy with a parent, a child (which will be watched), and a control child.
// Then it creates a watcher for the child and performs the following assertions:
// - The watcher is not notified when a sibling is asked for reboot.
// - The watcher is notified when the child is directly asked for reboot.
// - The watcher is notified when the parent is required for reboot.
// The tests are run using the watchertest harness.
func (s *watcherSuite) TestWatchMachineForReboot(c *tc.C) {
	// Create machine hierarchy to reboot from parent, with a child (which will be watched) and a control child
	parentUUID, err := s.svc.CreateMachine(context.Background(), "parent")
	c.Assert(err, tc.IsNil)
	childUUID, err := s.svc.CreateMachineWithParent(context.Background(), "child", "parent")
	c.Assert(err, tc.ErrorIsNil)
	controlUUID, err := s.svc.CreateMachineWithParent(context.Background(), "control", "parent")
	c.Assert(err, tc.ErrorIsNil)

	// Create watcher for child
	watcher, err := s.svc.WatchMachineReboot(context.Background(), childUUID)
	c.Assert(err, tc.IsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Ensure that the watcher is not notified when a sibling is asked for reboot
	harness.AddTest(func(c *tc.C) {
		err := s.svc.RequireMachineReboot(context.Background(), controlUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Ensure that the watcher is notified when the child is directly asked for reboot
	harness.AddTest(func(c *tc.C) {
		err := s.svc.RequireMachineReboot(context.Background(), childUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Ensure that the watcher is notified when the parent is required for reboot
	harness.AddTest(func(c *tc.C) {
		err := s.svc.RequireMachineReboot(context.Background(), parentUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Ensure that the watcher is not notified when a sibling is cleared from reboot
	harness.AddTest(func(c *tc.C) {
		err := s.svc.ClearMachineReboot(context.Background(), controlUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Ensure that the watcher is notified when the child is directly cleared from reboot
	harness.AddTest(func(c *tc.C) {
		err := s.svc.ClearMachineReboot(context.Background(), childUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Ensure that the watcher is notified when the parent is cleared from reboot
	harness.AddTest(func(c *tc.C) {
		err := s.svc.ClearMachineReboot(context.Background(), parentUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	harness.Run(c, struct{}{})
}

func uintptr(u uint64) *uint64 {
	return &u
}
