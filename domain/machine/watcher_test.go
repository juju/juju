// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"context"
	"testing"

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

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

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
		func(ctx context.Context) (service.Provider, error) {
			return service.NewNoopProvider(), nil
		},
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *watcherSuite) TestWatchModelMachines(c *tc.C) {
	_, err := s.svc.CreateMachine(c.Context(), "0", ptr("nonce-123"))
	c.Assert(err, tc.IsNil)

	_, err = s.svc.CreateMachine(c.Context(), "0/lxd/0", ptr("nonce-123"))
	c.Assert(err, tc.IsNil)

	s.AssertChangeStreamIdle(c)

	watcher, err := s.svc.WatchModelMachines(c.Context())
	c.Assert(err, tc.IsNil)
	defer watchertest.CleanKill(c, watcher)

	watcherC := watchertest.NewStringsWatcherC(c, watcher)

	// The initial event should have the machine we created prior,
	// but not the container.
	watcherC.AssertChange("0")

	// A new machine triggers an emission.
	_, err = s.svc.CreateMachine(c.Context(), "1", ptr("nonce-123"))
	c.Assert(err, tc.IsNil)
	watcherC.AssertChange("1")

	// An update triggers an emission.
	err = s.svc.SetMachineLife(c.Context(), "1", life.Dying)
	c.Assert(err, tc.IsNil)
	watcherC.AssertChange("1")

	// A deletion is ignored.
	err = s.svc.DeleteMachine(c.Context(), "1")
	c.Assert(err, tc.IsNil)

	// As is a container creation.
	_, err = s.svc.CreateMachine(c.Context(), "0/lxd/1", ptr("nonce-123"))
	c.Assert(err, tc.IsNil)

	s.AssertChangeStreamIdle(c)
	watcherC.AssertNoChange()
}

func (s *watcherSuite) TestMachineCloudInstanceWatchWithSet(c *tc.C) {
	// Create a machineUUID and set its cloud instance.
	machineUUID, err := s.svc.CreateMachine(c.Context(), "machine-1", ptr("nonce-123"))
	c.Assert(err, tc.IsNil)
	hc := &instance.HardwareCharacteristics{
		Mem:      ptr[uint64](1024),
		RootDisk: ptr[uint64](256),
		CpuCores: ptr[uint64](4),
		CpuPower: ptr[uint64](75),
	}
	watcher, err := s.svc.WatchMachineCloudInstances(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Should notify when the machine cloud instance is set.
	harness.AddTest(func(c *tc.C) {
		err = s.svc.SetMachineCloudInstance(c.Context(), machineUUID, "42", "", "nonce", hc)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestMachineCloudInstanceWatchWithDelete(c *tc.C) {
	// Create a machineUUID and set its cloud instance.
	machineUUID, err := s.svc.CreateMachine(c.Context(), "machine-1", ptr("nonce-123"))
	c.Assert(err, tc.IsNil)
	hc := &instance.HardwareCharacteristics{
		Mem:      ptr[uint64](1024),
		RootDisk: ptr[uint64](256),
		CpuCores: ptr[uint64](4),
		CpuPower: ptr[uint64](75),
	}
	err = s.svc.SetMachineCloudInstance(c.Context(), machineUUID, "42", "", "nonce", hc)
	c.Assert(err, tc.IsNil)

	watcher, err := s.svc.WatchMachineCloudInstances(c.Context(), machineUUID)
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Should notify when the machine cloud instance is deleted.
	harness.AddTest(func(c *tc.C) {
		err = s.svc.DeleteMachineCloudInstance(c.Context(), machineUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchLXDProfiles(c *tc.C) {
	machineUUIDm0, err := s.svc.CreateMachine(c.Context(), "machine-1", ptr("nonce-123"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.svc.SetMachineCloudInstance(c.Context(), machineUUIDm0, instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)
	machineUUIDm1, err := s.svc.CreateMachine(c.Context(), "machine-2", ptr("nonce-123"))
	c.Assert(err, tc.ErrorIsNil)
	err = s.svc.SetMachineCloudInstance(c.Context(), machineUUIDm1, instance.Id("456"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := s.svc.WatchLXDProfiles(c.Context(), machineUUIDm0)
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Should notify when a new profile is added.
	harness.AddTest(func(c *tc.C) {
		err := s.svc.SetAppliedLXDProfileNames(c.Context(), machineUUIDm0, []string{"profile-0"})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Should notify when profiles are overwritten.
	harness.AddTest(func(c *tc.C) {
		err := s.svc.SetAppliedLXDProfileNames(c.Context(), machineUUIDm0, []string{"profile-0", "profile-1", "profile-2"})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Nothing to notify when the lxd profiles are set on the other (non
	// watched) machine.
	harness.AddTest(func(c *tc.C) {
		err := s.svc.SetAppliedLXDProfileNames(c.Context(), machineUUIDm1, []string{"profile-0"})
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
	parentUUID, err := s.svc.CreateMachine(c.Context(), "parent", ptr("nonce-123"))
	c.Assert(err, tc.IsNil)
	childUUID, err := s.svc.CreateMachineWithParent(c.Context(), "child", "parent")
	c.Assert(err, tc.ErrorIsNil)
	controlUUID, err := s.svc.CreateMachineWithParent(c.Context(), "control", "parent")
	c.Assert(err, tc.ErrorIsNil)

	// Create watcher for child
	watcher, err := s.svc.WatchMachineReboot(c.Context(), childUUID)
	c.Assert(err, tc.IsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Ensure that the watcher is not notified when a sibling is asked for reboot
	harness.AddTest(func(c *tc.C) {
		err := s.svc.RequireMachineReboot(c.Context(), controlUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Ensure that the watcher is notified when the child is directly asked for reboot
	harness.AddTest(func(c *tc.C) {
		err := s.svc.RequireMachineReboot(c.Context(), childUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Ensure that the watcher is notified when the parent is required for reboot
	harness.AddTest(func(c *tc.C) {
		err := s.svc.RequireMachineReboot(c.Context(), parentUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Ensure that the watcher is not notified when a sibling is cleared from reboot
	harness.AddTest(func(c *tc.C) {
		err := s.svc.ClearMachineReboot(c.Context(), controlUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Ensure that the watcher is notified when the child is directly cleared from reboot
	harness.AddTest(func(c *tc.C) {
		err := s.svc.ClearMachineReboot(c.Context(), childUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Ensure that the watcher is notified when the parent is cleared from reboot
	harness.AddTest(func(c *tc.C) {
		err := s.svc.ClearMachineReboot(c.Context(), parentUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	harness.Run(c, struct{}{})
}

// TestWatchMachineLife tests the functionality of watching machine lifecycle
// changes.
func (s *watcherSuite) TestWatchMachineLife(c *tc.C) {
	watcher, err := s.svc.WatchMachineLife(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(func(c *tc.C) {
		_, err := s.svc.CreateMachine(c.Context(), "1", nil)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(func(c *tc.C) {
		_, err := s.svc.CreateMachine(c.Context(), "2", nil)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func ptr[T any](u T) *T {
	return &u
}
