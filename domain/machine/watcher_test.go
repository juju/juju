// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/changestream"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	applicationstorageservice "github.com/juju/juju/domain/application/service/storage"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/machine/state"
	objectstorestate "github.com/juju/juju/domain/objectstore/state"
	removalservice "github.com/juju/juju/domain/removal/service"
	removalstatecontroller "github.com/juju/juju/domain/removal/state/controller"
	removalstatemodel "github.com/juju/juju/domain/removal/state/model"
	"github.com/juju/juju/domain/storageprovisioning"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/environs"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	internalcharm "github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
	internaltesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
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

	modelUUID := uuid.MustNewUUID()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod",  "iaas", "test-model", "ec2")
		`, modelUUID.String(), internaltesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "machine")
	s.svc = service.NewWatchableService(
		state.NewState(
			func(ctx context.Context) (database.TxnRunner, error) { return factory(ctx) },
			clock.WallClock,
			loggertesting.WrapCheckLog(c),
		),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		func(ctx context.Context) (service.Provider, error) {
			return service.NewNoopProvider(), nil
		},
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *watcherSuite) TestWatchModelMachines(c *tc.C) {
	watcher, err := s.svc.WatchModelMachines(c.Context())
	c.Assert(err, tc.IsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	removalSt := removalstatemodel.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	// Should fire when a machine is created.
	var res0 service.AddMachineResults
	harness.AddTest(c, func(c *tc.C) {
		res0, err = s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
			Nonce: ptr("nonce-123"),
		})
		c.Assert(err, tc.IsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{res0.MachineName.String()}))
	})
	var res1 service.AddMachineResults
	harness.AddTest(c, func(c *tc.C) {
		res1, err = s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
			Nonce: ptr("nonce-123"),
		})
		c.Assert(err, tc.IsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{res1.MachineName.String()}))
	})
	// Should fire when the machine life changes.
	harness.AddTest(c, func(c *tc.C) {
		mUUID, err := s.svc.GetMachineUUID(c.Context(), res1.MachineName)
		c.Assert(err, tc.ErrorIsNil)
		_, err = removalSt.EnsureMachineNotAliveCascade(c.Context(), mUUID.String(), true)
		c.Assert(err, tc.IsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{res1.MachineName.String()}))
	})
	// Should not fire on containers.
	harness.AddTest(c, func(c *tc.C) {
		_, err = s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
			Directive: deployment.Placement{
				Type:      deployment.PlacementTypeContainer,
				Container: deployment.ContainerTypeLXD,
				Directive: res0.MachineName.String(),
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchModelMachinesInitialEventMachine(c *tc.C) {
	res0, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
		Nonce: ptr("nonce-123"),
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := s.svc.WatchModelMachines(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	var changes []string
	select {
	case changes = <-watcher.Changes():
	case <-c.Context().Done():
		c.Fatalf("watcher did not emit initial changes: %v", c.Context().Err())
	}

	// The machine appears in the initial changes.
	c.Assert(changes, tc.HasLen, 1)
	c.Assert(changes[0], tc.Equals, res0.MachineName.String())
}

func (s *watcherSuite) TestWatchModelMachinesInitialEventContainer(c *tc.C) {
	res, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := s.svc.WatchModelMachines(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	var changes []string
	select {
	case changes = <-watcher.Changes():
	case <-c.Context().Done():
		c.Fatalf("watcher did not emit initial changes: %v", c.Context().Err())
	}

	// Only the parent machine appears in the initial changes, not the
	// container.
	c.Assert(changes, tc.HasLen, 1)
	c.Assert(changes[0], tc.Equals, res.MachineName.String())
}

func (s *watcherSuite) TestWatchModelMachineLifeStartTimesInitialEvent(c *tc.C) {
	res0, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
		Nonce: ptr("nonce-123"),
	})
	c.Assert(err, tc.ErrorIsNil)

	s.AssertChangeStreamIdle(c)

	watcher, err := s.svc.WatchModelMachineLifeAndStartTimes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	watcherC := watchertest.NewStringsWatcherC(c, watcher)

	watcherC.AssertChange(res0.MachineName.String())
}

func (s *watcherSuite) TestWatchModelMachineLifeStartTimes(c *tc.C) {
	removalSt := removalstatemodel.NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	watcher, err := s.svc.WatchModelMachineLifeAndStartTimes(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	var res service.AddMachineResults
	harness.AddTest(c, func(c *tc.C) {
		var err error
		res, err = s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
			Nonce: ptr("nonce-123"),
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{res.MachineName.String()}))
	})

	harness.AddTest(c, func(c *tc.C) {
		mUUID, err := s.svc.GetMachineUUID(c.Context(), res.MachineName)
		c.Assert(err, tc.ErrorIsNil)
		_, err = removalSt.EnsureMachineNotAliveCascade(c.Context(), mUUID.String(), true)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{res.MachineName.String()}))
	})

	harness.AddTest(c, func(c *tc.C) {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, "UPDATE machine SET agent_started_at = DATETIME('2022-02-02')")
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"0"}))
	})

	harness.Run(c, nil)
}

func (s *watcherSuite) TestMachineCloudInstanceWatchWithSet(c *tc.C) {
	// Create a machineUUID and set its cloud instance.
	res, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
		Nonce: ptr("nonce-123"),
	})
	c.Assert(err, tc.IsNil)
	machineUUID, err := s.svc.GetMachineUUID(c.Context(), res.MachineName)
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
	harness.AddTest(c, func(c *tc.C) {
		err = s.svc.SetMachineCloudInstance(c.Context(), machineUUID, "42", "", "nonce", hc)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchLXDProfiles(c *tc.C) {
	res0, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUIDm0, err := s.svc.GetMachineUUID(c.Context(), res0.MachineName)
	c.Assert(err, tc.IsNil)
	err = s.svc.SetMachineCloudInstance(c.Context(), machineUUIDm0, instance.Id("123"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	res1, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUIDm1, err := s.svc.GetMachineUUID(c.Context(), res1.MachineName)
	c.Assert(err, tc.IsNil)
	err = s.svc.SetMachineCloudInstance(c.Context(), machineUUIDm1, instance.Id("456"), "", "nonce", nil)
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := s.svc.WatchLXDProfiles(c.Context(), machineUUIDm0)
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Should notify when a new profile is added.
	harness.AddTest(c, func(c *tc.C) {
		err := s.svc.SetAppliedLXDProfileNames(c.Context(), machineUUIDm0, []string{"profile-0"})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Should notify when profiles are overwritten.
	harness.AddTest(c, func(c *tc.C) {
		err := s.svc.SetAppliedLXDProfileNames(c.Context(), machineUUIDm0, []string{"profile-0", "profile-1", "profile-2"})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Nothing to notify when the lxd profiles are set on the other (non
	// watched) machine.
	harness.AddTest(c, func(c *tc.C) {
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
	res, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	parentUUID, err := s.svc.GetMachineUUID(c.Context(), res.MachineName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.ChildMachineName, tc.NotNil)
	childUUID, err := s.svc.GetMachineUUID(c.Context(), *res.ChildMachineName)
	c.Assert(err, tc.ErrorIsNil)
	controlContainerNames, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
			Directive: "0",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(controlContainerNames.ChildMachineName, tc.NotNil)
	controlUUID, err := s.svc.GetMachineUUID(c.Context(), *controlContainerNames.ChildMachineName)
	c.Assert(err, tc.ErrorIsNil)

	// Create watcher for child
	watcher, err := s.svc.WatchMachineReboot(c.Context(), childUUID)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Ensure that the watcher is not notified when a sibling is asked for reboot
	harness.AddTest(c, func(c *tc.C) {
		err := s.svc.RequireMachineReboot(c.Context(), controlUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Ensure that the watcher is notified when the child is directly asked for reboot
	harness.AddTest(c, func(c *tc.C) {
		err := s.svc.RequireMachineReboot(c.Context(), childUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Ensure that the watcher is notified when the parent is required for reboot
	harness.AddTest(c, func(c *tc.C) {
		err := s.svc.RequireMachineReboot(c.Context(), parentUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Ensure that the watcher is not notified when a sibling is cleared from reboot
	harness.AddTest(c, func(c *tc.C) {
		err := s.svc.ClearMachineReboot(c.Context(), controlUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Ensure that the watcher is notified when the child is directly cleared from reboot
	harness.AddTest(c, func(c *tc.C) {
		err := s.svc.ClearMachineReboot(c.Context(), childUUID)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// Ensure that the watcher is notified when the parent is cleared from reboot
	harness.AddTest(c, func(c *tc.C) {
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
	watcher, err := s.svc.WatchMachineLife(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		_, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// Create a second machine, make sure it doesn't trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		_, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchMachineLifeAndDependantsNotFound(c *tc.C) {
	_, err := s.svc.WatchMachineLifeAndDependants(c.Context(), "0")
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestWatchMachineLifeAndDependants tests the functionality of watching machine
// lifecycle changes and lifecycle/deletion of dependants.
func (s *watcherSuite) TestWatchMachineLifeAndDependants(c *tc.C) {
	m, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	s.AssertChangeStreamIdle(c)

	watcher, err := s.svc.WatchMachineLifeAndDependants(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	// Create a second machine, make sure it doesn't trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		_, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// Add a container and make sure a change is seen.
	harness.AddTest(c, func(c *tc.C) {
		_, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
			Directive: deployment.Placement{
				Type:      deployment.PlacementTypeContainer,
				Container: deployment.ContainerTypeLXD,
				Directive: m.MachineName.String(),
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

// TestWatchMachineLifeAndDependantsWithUnits tests the functionality of
// watching machine lifecycle changes and lifecycle/deletion of dependants.
func (s *watcherSuite) TestWatchMachineLifeAndDependantsWithUnits(c *tc.C) {
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "pelican")
	appService := s.setupApplicationService(c, factory)
	removalService := s.setupRemovalService(c, factory)

	appUUID := s.createIAASApplication(c, appService, "some-app", applicationservice.AddIAASUnitArg{})
	unitUUIDs, _ := s.getAppUnitAndMachineUUIDs(c, appUUID)
	unitUUID := unitUUIDs[0]

	s.AssertChangeStreamIdle(c)

	watcher, err := s.svc.WatchMachineLifeAndDependants(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	harness.AddTest(c, func(c *tc.C) {
		// Dump another unit on the same machine, which will prevent the
		// removal of the machine when the unit is removed.
		_, _, err := appService.AddIAASUnits(c.Context(), "some-app", applicationservice.AddIAASUnitArg{
			AddUnitArg: applicationservice.AddUnitArg{
				Placement: &instance.Placement{Scope: instance.MachineScope, Directive: "0"},
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		// Removing the unit should trigger a change
		_, err := removalService.RemoveUnit(c.Context(), unit.UUID(unitUUID), false, false, 0)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		appUUID = s.createIAASApplication(c, appService, "other-app", applicationservice.AddIAASUnitArg{})
		c.Assert(err, tc.ErrorIsNil)

		unitUUIDs, _ := s.getAppUnitAndMachineUUIDs(c, appUUID)
		unitUUID = unitUUIDs[0]
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		// Removing the unit should trigger a change
		_, err := removalService.RemoveUnit(c.Context(), unit.UUID(unitUUID), false, false, 0)
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}

// TestWatchMachineLifeAndDependantsWithStorage tests the functionality of
// watching machine lifecycle changes and lifecycle/deletion of dependants.
func (s *watcherSuite) TestWatchMachineLifeAndDependantsWithStorage(c *tc.C) {
	m, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	mUUID, err := s.svc.GetMachineUUID(c.Context(), m.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	mfsUUID := s.createMachineFilesystem(c, mUUID.String())
	mvUUID := s.createMachineVolume(c, mUUID.String())
	fsUUID := s.createAttachedFilesystem(c, mUUID.String())
	vUUID := s.createAttachedVolume(c, mUUID.String())
	pvUUID := s.createPlanAttachedVolume(c, mUUID.String())

	s.AssertChangeStreamIdle(c)

	watcher, err := s.svc.WatchMachineLifeAndDependants(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	harness.AddTest(c, func(c *tc.C) {
		s.deleteFilesystem(c, mfsUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		s.deleteFilesystem(c, fsUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		s.deleteVolume(c, mvUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		s.deleteVolume(c, vUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		s.deleteVolume(c, pvUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchMachineContainerLifeInit(c *tc.C) {
	watcher, err := s.svc.WatchMachineContainerLife(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)

	var changes []string
	select {
	case changes = <-watcher.Changes():
	case <-c.Context().Done():
		c.Fatalf("watcher did not emit initial changes: %v", c.Context().Err())
	}

	c.Assert(changes, tc.HasLen, 0)
}

func (s *watcherSuite) TestWatchMachineContainerLifeInitMachine(c *tc.C) {
	res, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := s.svc.WatchMachineContainerLife(c.Context(), res.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	var changes []string
	select {
	case changes = <-watcher.Changes():
	case <-c.Context().Done():
		c.Fatalf("watcher did not emit initial changes: %v", c.Context().Err())
	}

	c.Assert(changes, tc.HasLen, 0)
}

func (s *watcherSuite) TestWatchMachineContainerLifeInitMachineContainer(c *tc.C) {
	res, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
		Directive: deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := s.svc.WatchMachineContainerLife(c.Context(), res.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	var changes []string
	select {
	case changes = <-watcher.Changes():
	case <-c.Context().Done():
		c.Fatalf("watcher did not emit initial changes: %v", c.Context().Err())
	}

	c.Assert(changes, tc.HasLen, 1)
	c.Assert(changes[0], tc.Equals, res.MachineName.String()+"/lxd/0")
}

func (s *watcherSuite) TestWatchMachineContainerLife(c *tc.C) {
	res0, err := s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	watcher, err := s.svc.WatchMachineContainerLife(c.Context(), res0.MachineName)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	harness.AddTest(c, func(c *tc.C) {
		_, err = s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		_, err = s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
			Directive: deployment.Placement{
				Type:      deployment.PlacementTypeContainer,
				Container: deployment.ContainerTypeLXD,
				Directive: res0.MachineName.String(),
			},
		})
		c.Assert(err, tc.ErrorIsNil)

	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertChange()
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchMachineContainerLifeNoDispatch(c *tc.C) {
	watcher, err := s.svc.WatchMachineContainerLife(c.Context(), "1")
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		var err error
		_, err = s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		_, err = s.svc.AddMachine(c.Context(), domainmachine.AddMachineArgs{
			Platform: deployment.Platform{
				Channel: "24.04",
				OSType:  deployment.Ubuntu,
			},
			Directive: deployment.Placement{
				Type:      deployment.PlacementTypeContainer,
				Container: deployment.ContainerTypeLXD,
				Directive: "0",
			},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) setupRemovalService(c *tc.C, factory domain.WatchableDBFactory) *removalservice.WatchableService {
	log := loggertesting.WrapCheckLog(c)

	modelState := removalstatemodel.NewState(func(ctx context.Context) (database.TxnRunner, error) { return s.ModelTxnRunner(), nil }, log)
	svc := removalservice.NewWatchableService(
		removalstatecontroller.NewState(func(ctx context.Context) (database.TxnRunner, error) { return s.NoopTxnRunner(), nil }, log),
		modelState,
		domain.NewWatcherFactory(factory, log),
		nil,
		nil,
		model.UUID(s.ModelUUID()),
		clock.WallClock,
		log,
	)

	return svc
}

func (s *watcherSuite) setupApplicationService(c *tc.C, factory domain.WatchableDBFactory) *applicationservice.WatchableService {
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	providerGetter := func(ctx context.Context) (applicationservice.Provider, error) {
		return appProvider{}, nil
	}
	caasProviderGetter := func(ctx context.Context) (applicationservice.CAASProvider, error) {
		return appProvider{}, nil
	}
	storageProviderRegistryGetter := corestorage.ConstModelStorageRegistry(
		func() internalstorage.ProviderRegistry {
			return internalstorage.NotImplementedProviderRegistry{}
		},
	)
	state := applicationstate.NewState(modelDB, clock.WallClock, loggertesting.WrapCheckLog(c))
	storageSvc := applicationstorageservice.NewService(
		state,
		applicationstorageservice.NewStoragePoolProvider(
			storageProviderRegistryGetter, state,
		),
		loggertesting.WrapCheckLog(c),
	)

	return applicationservice.NewWatchableService(
		state,
		storageSvc,
		domaintesting.NoopLeaderEnsurer(),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		nil,
		providerGetter,
		caasProviderGetter,
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *watcherSuite) createIAASApplication(c *tc.C, svc *applicationservice.WatchableService, name string, units ...applicationservice.AddIAASUnitArg) string {
	ch := &stubCharm{name: "test-charm"}
	appID, err := svc.CreateIAASApplication(c.Context(), name, ch, corecharm.Origin{
		Source: corecharm.CharmHub,
		Platform: corecharm.Platform{
			Channel:      "24.04",
			OS:           "ubuntu",
			Architecture: "amd64",
		},
	}, applicationservice.AddApplicationArgs{
		ReferenceName: name,
		DownloadInfo: &charm.DownloadInfo{
			Provenance:  charm.ProvenanceDownload,
			DownloadURL: "http://example.com",
		},
		ResolvedResources: applicationservice.ResolvedResources{{
			Name:     "buzz",
			Revision: ptr(42),
			Origin:   charmresource.OriginStore,
		}},
	}, units...)
	c.Assert(err, tc.ErrorIsNil)

	s.setCharmObjectStoreMetadata(c, appID.String())

	return appID.String()
}

func (s *watcherSuite) setCharmObjectStoreMetadata(c *tc.C, appID string) {
	modelDB := func(ctx context.Context) (database.TxnRunner, error) {
		return s.ModelTxnRunner(), nil
	}

	objectStoreUUID, err := objectstorestate.NewState(modelDB).PutMetadata(c.Context(), coreobjectstore.Metadata{
		SHA256: fmt.Sprintf("%v-sha256", appID),
		SHA384: fmt.Sprintf("%v-sha384", appID),
		Path:   fmt.Sprintf("/path/to/%v", appID),
		Size:   100,
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE charm
SET object_store_uuid = ?
WHERE uuid IN (
	SELECT charm_uuid
	FROM application
	WHERE uuid = ?
)`, objectStoreUUID, appID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) getAppUnitAndMachineUUIDs(c *tc.C, appUUID string) (units []string, machines []string) {
	result := make(map[string]string)
	err := s.ModelTxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT u.uuid, m.uuid
FROM unit AS u
JOIN net_node AS nn ON nn.uuid = u.net_node_uuid
JOIN machine AS m ON m.net_node_uuid = nn.uuid
WHERE u.application_uuid = ?
`, appUUID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				unitUUID    string
				machineUUID string
			)
			if err := rows.Scan(&unitUUID, &machineUUID); err != nil {
				return err
			}
			result[unitUUID] = machineUUID
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)

	var allUnitUUIDs []string
	var allMachineUUIDs []string
	for unitUUID, machineUUID := range result {
		allUnitUUIDs = append(allUnitUUIDs, unitUUID)

		// If the machine UUID is empty, it means that the unit is not
		// associated with any machine.
		if machineUUID == "" {
			continue
		}
		allMachineUUIDs = append(allMachineUUIDs, machineUUID)
	}

	return allUnitUUIDs, allMachineUUIDs
}

func (s *watcherSuite) deleteFilesystem(c *tc.C, fsUUID string) {
	txn := func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
DELETE FROM machine_filesystem WHERE filesystem_uuid = ?
			`, fsUUID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
DELETE FROM storage_filesystem_attachment WHERE storage_filesystem_uuid = ?
			`, fsUUID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
DELETE FROM storage_filesystem WHERE uuid = ?
			`, fsUUID)
		if err != nil {
			return err
		}
		return nil
	}
	err := s.ModelTxnRunner().StdTxn(c.Context(), txn)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) deleteVolume(c *tc.C, volUUID string) {
	txn := func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
DELETE FROM machine_volume WHERE volume_uuid = ?
			`, volUUID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
DELETE FROM storage_volume_attachment WHERE storage_volume_uuid = ?
			`, volUUID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
DELETE FROM storage_volume_attachment_plan WHERE storage_volume_uuid = ?
			`, volUUID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
DELETE FROM storage_volume WHERE uuid = ?
			`, volUUID)
		if err != nil {
			return err
		}
		return nil
	}
	err := s.ModelTxnRunner().StdTxn(c.Context(), txn)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) createMachineFilesystem(
	c *tc.C, machineUUID string,
) string {
	fsUUID := tc.Must(c, storageprovisioning.NewFilesystemUUID).String()
	txn := func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id) VALUES (?, ?, ?, ?)
		`, fsUUID, "0", 0, 0)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO machine_filesystem (machine_uuid, filesystem_uuid) VALUES (?, ?)
		`, machineUUID, fsUUID)
		if err != nil {
			return err
		}
		return nil
	}
	err := s.ModelTxnRunner().StdTxn(c.Context(), txn)
	c.Assert(err, tc.ErrorIsNil)
	return fsUUID
}

func (s *watcherSuite) createMachineVolume(
	c *tc.C, machineUUID string,
) string {
	volUUID := tc.Must(c, storageprovisioning.NewVolumeUUID).String()
	txn := func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id) VALUES (?, ?, ?, ?)
		`, volUUID, "0", 0, 0)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO machine_volume (machine_uuid, volume_uuid) VALUES (?, ?)
		`, machineUUID, volUUID)
		if err != nil {
			return err
		}
		return nil
	}
	err := s.ModelTxnRunner().StdTxn(c.Context(), txn)
	c.Assert(err, tc.ErrorIsNil)
	return volUUID
}

func (s *watcherSuite) createAttachedVolume(
	c *tc.C, machineUUID string,
) string {
	volUUID := tc.Must(c, storageprovisioning.NewVolumeUUID).String()
	vaUUID := tc.Must(c, storageprovisioning.NewVolumeAttachmentUUID).String()
	txn := func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, ?, ?)
		`, volUUID, "1", 0, 0)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO storage_volume_attachment (uuid, storage_volume_uuid, life_id,
                                       provision_scope_id, net_node_uuid)
VALUES (?, ?, ?, ?, (SELECT net_node_uuid FROM machine WHERE uuid = ?))
		`, vaUUID, volUUID, 0, 0, machineUUID)
		if err != nil {
			return err
		}
		return nil
	}
	err := s.ModelTxnRunner().StdTxn(c.Context(), txn)
	c.Assert(err, tc.ErrorIsNil)
	return volUUID
}

func (s *watcherSuite) createPlanAttachedVolume(
	c *tc.C, machineUUID string,
) string {
	volUUID := tc.Must(c, storageprovisioning.NewVolumeUUID).String()
	vaUUID := tc.Must(c, storageprovisioning.NewVolumeAttachmentUUID).String()
	txn := func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, ?, ?)
		`, volUUID, "2", 0, 0)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO storage_volume_attachment_plan (uuid, storage_volume_uuid, life_id,
                                            provision_scope_id, net_node_uuid)
VALUES (?, ?, ?, ?, (SELECT net_node_uuid FROM machine WHERE uuid = ?))
		`, vaUUID, volUUID, 0, 0, machineUUID)
		if err != nil {
			return err
		}
		return nil
	}
	err := s.ModelTxnRunner().StdTxn(c.Context(), txn)
	c.Assert(err, tc.ErrorIsNil)
	return volUUID
}

func (s *watcherSuite) createAttachedFilesystem(
	c *tc.C, machineUUID string,
) string {
	fsUUID := tc.Must(c, storageprovisioning.NewFilesystemUUID).String()
	faUUID := tc.Must(c, storageprovisioning.NewFilesystemAttachmentUUID).String()
	txn := func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, ?, ?)
		`, fsUUID, "1", 0, 0)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO storage_filesystem_attachment (uuid, storage_filesystem_uuid,
                                          life_id, provision_scope_id,
                                          net_node_uuid)
VALUES (?, ?, ?, ?, (SELECT net_node_uuid FROM machine WHERE uuid = ?))
		`, faUUID, fsUUID, 0, 0, machineUUID)
		if err != nil {
			return err
		}
		return nil
	}
	err := s.ModelTxnRunner().StdTxn(c.Context(), txn)
	c.Assert(err, tc.ErrorIsNil)
	return fsUUID
}

type stubCharm struct {
	name        string
	subordinate bool
}

func (s *stubCharm) Meta() *internalcharm.Meta {
	name := s.name
	if name == "" {
		name = "test"
	}
	return &internalcharm.Meta{
		Name:        name,
		Subordinate: s.subordinate,
		Resources: map[string]charmresource.Meta{
			"buzz": {
				Name:        "buzz",
				Type:        charmresource.TypeFile,
				Path:        "/path/to/buzz.tgz",
				Description: "buzz description",
			},
		},
	}
}

func (s *stubCharm) Manifest() *internalcharm.Manifest {
	return &internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name: "ubuntu",
			Channel: internalcharm.Channel{
				Risk: internalcharm.Stable,
			},
			Architectures: []string{"amd64"},
		}},
	}
}

func (s *stubCharm) Config() *internalcharm.ConfigSpec {
	return &internalcharm.ConfigSpec{
		Options: map[string]internalcharm.Option{
			"foo": {
				Type:    "string",
				Default: "bar",
			},
		},
	}
}

func (s *stubCharm) Actions() *internalcharm.Actions {
	return nil
}

func (s *stubCharm) Revision() int {
	return 0
}

func (s *stubCharm) Version() string {
	return ""
}

type appProvider struct {
	applicationservice.Provider
	applicationservice.CAASProvider
}

func (appProvider) PrecheckInstance(ctx context.Context, args environs.PrecheckInstanceParams) error {
	return nil
}

func (appProvider) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	return constraints.NewValidator(), nil
}

func (appProvider) Application(string, caas.DeploymentType) caas.Application {
	return &caasApplication{}
}

type caasApplication struct {
	caas.Application
}

func (caasApplication) Units() ([]caas.Unit, error) {
	return []caas.Unit{{
		Id: "some-app-0",
	}}, nil
}

func ptr[T any](v T) *T {
	return &v
}
