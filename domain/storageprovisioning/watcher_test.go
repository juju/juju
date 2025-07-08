// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning_test

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/storageprovisioning/service"
	"github.com/juju/juju/domain/storageprovisioning/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

// watcherSuite is a set of tests for asserting the public watcher interface
// exposed by this domain.
type watcherSuite struct {
	changestreamtesting.ModelSuite
}

// TestWatcherSuite runs the tests that are apart of [watcherSuite].
func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

// TestWatchMachineProvisionedFilesystems asserts the watcher behaviour for
// machine provisioned filesystems through both the service and state layers.
func (s *watcherSuite) TestWatchMachineProvisionedFilesystems(c *tc.C) {
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "storage"),
		loggertesting.WrapCheckLog(c),
	)

	machineUUID := s.newMachine(c)

	svc := service.NewService(state.NewState(s.TxnRunnerFactory()), factory)
	watcher, err := svc.WatchMachineProvisionedFilesystems(c.Context(), coremachine.UUID(machineUUID))
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	var (
		fsOneUUID, fsOneID, fsTwoUUID, fsTwoID string
		fsaTwoUUID                             string
	)

	// Assert that without any attachments machine provisioned filesystems do
	// not emit a change event on the watcher until at least one attachment
	// exists.
	harness.AddTest(func(c *tc.C) {
		fsOneUUID, fsOneID = s.newMachineFilesystem(c)
		fsTwoUUID, fsTwoID = s.newMachineFilesystem(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that adding the first attachment for the filesystem causes the
	// watcher to fire.
	harness.AddTest(func(c *tc.C) {
		s.newMachineFilesystemAttachmentForMachine(c, fsOneUUID, machineUUID)
		fsaTwoUUID = s.newMachineFilesystemAttachmentForMachine(c, fsTwoUUID, machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsOneID, fsTwoID),
		)
	})

	// Assert that a life change to a filesystem is reported by the watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeFilesystemLife(c, fsOneUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsOneID),
		)
	})

	// Assert that changing something about a filesystem which isn't the life
	// does not produce a change in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeFilesystemProviderID(c, fsTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting the last filesystem attachment for a filesystem
	// results in the watcher firing.
	harness.AddTest(func(c *tc.C) {
		s.deleteFilesystemAttachment(c, fsaTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsTwoID),
		)
	})

	harness.Run(c, []string(nil))
}

// TestWatchMachineProvisionedFilesystemAttachments asserts the watcher
// behaviour for filesystem attachments that are machine provisioned through
// both the service and state layers.
func (s *watcherSuite) TestWatchMachineProvisionedFilesystemAttachments(c *tc.C) {
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "storage"),
		loggertesting.WrapCheckLog(c),
	)

	machineUUID := s.newMachine(c)

	svc := service.NewService(state.NewState(s.TxnRunnerFactory()), factory)
	watcher, err := svc.WatchMachineProvisionedFilesystemAttachments(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	var (
		changeVals    []string
		fsaChangeUUID string
	)

	// Assert new machine provisioned filesystem attachments come out in the
	// watcher.
	harness.AddTest(func(c *tc.C) {
		fsOneUUID, _ := s.newModelFilesystem(c)
		fsTwoUUID, _ := s.newModelFilesystem(c)
		fsaOneUUID := s.newMachineFilesystemAttachmentForMachine(c, fsOneUUID, machineUUID)
		fsaChangeUUID = s.newMachineFilesystemAttachmentForMachine(c, fsTwoUUID, machineUUID)

		changeVals = []string{fsaOneUUID, fsaChangeUUID}
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(changeVals...),
		)
	})

	// Assert that a life change to a filesystem attachment is reported in the
	// watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeFilesystemAttachmentLife(c, fsaChangeUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsaChangeUUID),
		)
	})

	// Assert that changing something about a filesystem attachment which isn't
	// the life does not produce a change in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeFilesystemAttachmentMountPoint(c, fsaChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting a filesystem attachemtn is reported in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.deleteFilesystemAttachment(c, fsaChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsaChangeUUID),
		)
	})

	harness.Run(c, []string(nil))
}

// TestWatchModelProvisionedFilesystemAttachments asserts the watcher behaviour
// for filesystem attachments that are model provisioned through both the
// service and state layers.
func (s *watcherSuite) TestWatchModelProvisionedFilesystemAttachments(c *tc.C) {
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "storage"),
		loggertesting.WrapCheckLog(c),
	)

	machineUUID := s.newMachine(c)

	svc := service.NewService(state.NewState(s.TxnRunnerFactory()), factory)
	watcher, err := svc.WatchModelProvisionedFilesystemAttachments(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	var (
		changeVals    []string
		fsaChangeUUID string
	)

	// Assert new model provisioned filesystems come out in the watcher.
	harness.AddTest(func(c *tc.C) {
		fsOneUUID, _ := s.newModelFilesystem(c)
		fsTwoUUID, _ := s.newModelFilesystem(c)
		fsaOneUUID := s.newModelFilesystemAttachmentForMachine(c, fsOneUUID, machineUUID)
		fsaChangeUUID = s.newModelFilesystemAttachmentForMachine(c, fsTwoUUID, machineUUID)

		changeVals = []string{fsaOneUUID, fsaChangeUUID}
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(changeVals...),
		)
	})

	// Assert that a life change to a filesystem attachment is reported in the
	// watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeFilesystemAttachmentLife(c, fsaChangeUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsaChangeUUID),
		)
	})

	// Assert that changing something about a filesystem attachment which isn't
	// the life does not produce a change in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeFilesystemAttachmentMountPoint(c, fsaChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting a filesystem attachment is reported in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.deleteFilesystemAttachment(c, fsaChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsaChangeUUID),
		)
	})

	harness.Run(c, []string{})
}

// TestWatchModelProvisionedFilesystems asserts the watcher behaviour for
// filesystems that are model provisioned through both the service and state
// layers.
func (s *watcherSuite) TestWatchModelProvisionedFilesystems(c *tc.C) {
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "storage"),
		loggertesting.WrapCheckLog(c),
	)

	svc := service.NewService(state.NewState(s.TxnRunnerFactory()), factory)
	watcher, err := svc.WatchModelProvisionedFilesystems(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	var (
		changeVals   []string
		fsChangeUUID string
		fsChangeID   string
	)

	// Assert new model provisioned filesystems come out in the watcher.
	harness.AddTest(func(c *tc.C) {
		_, fsOneID := s.newModelFilesystem(c)
		fsChangeUUID, fsChangeID = s.newModelFilesystem(c)
		changeVals = []string{fsOneID, fsChangeID}
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(changeVals...),
		)
	})

	// Assert that a life change to a filesystem is reported in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeFilesystemLife(c, fsChangeUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsChangeID),
		)
	})

	// Assert that changing something about a filesystem which isn't the life
	// does not produce a change in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeFilesystemProviderID(c, fsChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting a filesystem is reported in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.deleteFilesystem(c, fsChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsChangeID),
		)
	})

	harness.Run(c, []string{})
}

// TestWatchMachineProvisionedVolumes asserts the watcher behaviour for
// volumes that are machine provisioned through both the service and state
// layers.
func (s *watcherSuite) TestWatchMachineProvisionedVolumes(c *tc.C) {
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "storage"),
		loggertesting.WrapCheckLog(c),
	)

	machineUUID := s.newMachine(c)

	svc := service.NewService(state.NewState(s.TxnRunnerFactory()), factory)
	watcher, err := svc.WatchMachineProvisionedVolumes(c.Context(), coremachine.UUID(machineUUID))
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	var (
		vsOneUUID, vsOneID, vsTwoUUID, vsTwoID string
		vsaTwoUUID                             string
	)

	// Assert that without any attachments machine provisioned volumes do
	// not emit a change event on the watcher until at least one attachment
	// exists.
	harness.AddTest(func(c *tc.C) {
		vsOneUUID, vsOneID = s.newMachineVolume(c)
		vsTwoUUID, vsTwoID = s.newMachineVolume(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that adding the first attachment for the volume causes the
	// watcher to fire.
	harness.AddTest(func(c *tc.C) {
		s.newMachineVolumeAttachmentForMachine(c, vsOneUUID, machineUUID)
		vsaTwoUUID = s.newMachineVolumeAttachmentForMachine(c, vsTwoUUID, machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsOneID, vsTwoID),
		)
	})

	// Assert that a life change to a volume is reported by the watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeVolumeLife(c, vsOneUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsOneID),
		)
	})

	// Assert that changing something about a volume which isn't the life
	// does not produce a change in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeVolumeProviderID(c, vsTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting the last volume attachment for a volume results in
	// the watcher firing.
	harness.AddTest(func(c *tc.C) {
		s.deleteVolumeAttachment(c, vsaTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsTwoID),
		)
	})

	harness.Run(c, []string(nil))
}

// TestWatchMachineProvisionedVolumeAttachments asserts the watcher
// behaviour for volume attachments provisioned by machines through both the
// service and state layers.
func (s *watcherSuite) TestWatchMachineProvisionedVolumeAttachments(c *tc.C) {
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "storage"),
		loggertesting.WrapCheckLog(c),
	)

	machineUUID := s.newMachine(c)

	svc := service.NewService(state.NewState(s.TxnRunnerFactory()), factory)
	watcher, err := svc.WatchMachineProvisionedVolumeAttachments(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	var (
		changeVals    []string
		vsaChangeUUID string
	)

	// Assert new machine provisioned volume attachment comes out in the
	// watcher.
	harness.AddTest(func(c *tc.C) {
		vsOneUUID, _ := s.newMachineVolume(c)
		vsTwoUUID, _ := s.newMachineVolume(c)
		vsaOneUUID := s.newMachineVolumeAttachmentForMachine(c, vsOneUUID, machineUUID)
		vsaChangeUUID = s.newMachineVolumeAttachmentForMachine(c, vsTwoUUID, machineUUID)

		changeVals = []string{vsaOneUUID, vsaChangeUUID}
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(changeVals...),
		)
	})

	// Assert that a life change to a volume attachment is reported in the
	// watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeVolumeAttchmentLife(c, vsaChangeUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsaChangeUUID),
		)
	})

	// Assert that changing something about a volume attachment which isn't
	// the life does not produce a change in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeVolumeAttachmentReadOnly(c, vsaChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting a volume attachment is reported in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.deleteVolumeAttachment(c, vsaChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsaChangeUUID),
		)
	})

	harness.Run(c, []string(nil))
}

// TestWatchModelProvisionedVolumeAttachments asserts the watcher behaviour
// for volume attachments that are model provisioned through both the service
// and state layers.
func (s *watcherSuite) TestWatchModelProvisionedVolumeAttachments(c *tc.C) {
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "storage"),
		loggertesting.WrapCheckLog(c),
	)

	machineUUID := s.newMachine(c)

	svc := service.NewService(state.NewState(s.TxnRunnerFactory()), factory)
	watcher, err := svc.WatchModelProvisionedVolumeAttachments(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	var (
		changeVals    []string
		vsaChangeUUID string
	)

	// Assert new model provisioned volume attachments come out in the watcher.
	harness.AddTest(func(c *tc.C) {
		vsOneUUID, _ := s.newModelVolume(c)
		vsTwoUUID, _ := s.newModelVolume(c)
		vsaOneUUID := s.newModelVolumeAttachmentForMachine(c, vsOneUUID, machineUUID)
		vsaChangeUUID = s.newModelVolumeAttachmentForMachine(c, vsTwoUUID, machineUUID)

		changeVals = []string{vsaOneUUID, vsaChangeUUID}
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(changeVals...),
		)
	})

	// Assert that a life change to a volume attachment is reported in the
	// watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeVolumeAttchmentLife(c, vsaChangeUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsaChangeUUID),
		)
	})

	// Assert that changing something about a volume attachment which isn't
	// the life does not produce a change in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeVolumeAttachmentReadOnly(c, vsaChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting a volume attachment is reported in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.deleteVolumeAttachment(c, vsaChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsaChangeUUID),
		)
	})

	harness.Run(c, []string{})
}

// TestWatchModelProvisionedVolumes asserts the watcher behaviour for
// volumes that are model provisioned through both the service and state layers.
func (s *watcherSuite) TestWatchModelProvisionedVolumes(c *tc.C) {
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "storage"),
		loggertesting.WrapCheckLog(c),
	)

	svc := service.NewService(state.NewState(s.TxnRunnerFactory()), factory)
	watcher, err := svc.WatchModelProvisionedVolumes(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	var (
		changeVals   []string
		vsChangeUUID string
		vsChangeID   string
	)

	// Assert new model provisioned volumes come out in the watcher.
	harness.AddTest(func(c *tc.C) {
		_, fsOneID := s.newModelVolume(c)
		vsChangeUUID, vsChangeID = s.newModelVolume(c)
		changeVals = []string{fsOneID, vsChangeID}
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(changeVals...),
		)
	})

	// Assert that a life change to a volume is reported in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeVolumeLife(c, vsChangeUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsChangeID),
		)
	})

	// Assert that changing something about a volume which isn't the life
	// does not produce a change in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeVolumeProviderID(c, vsChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting a volume is reported in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.deleteVolume(c, vsChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsChangeID),
		)
	})

	harness.Run(c, []string{})
}

// TestWatchVolumeAttachmentPlans asserts the watcher behaviour for volume
// attachment plans through both the service and state layers.
func (s *watcherSuite) TestWatchVolumeAttachmentPlans(c *tc.C) {
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "storage"),
		loggertesting.WrapCheckLog(c),
	)

	machineUUID := s.newMachine(c)

	svc := service.NewService(state.NewState(s.TxnRunnerFactory()), factory)
	watcher, err := svc.WatchVolumeAttachmentPlans(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	var (
		changeVals    []string
		vapChangeUUID string
		vsChangeID    string
	)

	// Assert new volume attachment plans come out in the watcher.
	harness.AddTest(func(c *tc.C) {
		vsOneUUID, vsOneID := s.newMachineVolume(c)
		vsTwoUUID, vsTwoID := s.newMachineVolume(c)
		s.newVolumeAttachmentPlanForMachine(c, vsOneUUID, machineUUID)
		vapTwoUUID := s.newVolumeAttachmentPlanForMachine(c, vsTwoUUID, machineUUID)

		// We expect that the volume attchment plan watcher outputs volume ids.
		changeVals = []string{vsOneID, vsTwoID}
		vapChangeUUID = vapTwoUUID
		vsChangeID = vsTwoID
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(changeVals...),
		)
	})

	// Assert that a life change to a volume attachment plan is reported in the
	// watcher.
	harness.AddTest(func(c *tc.C) {
		s.changeVolumeAttchmentPlanLife(c, vapChangeUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsChangeID),
		)
	})

	// Assert that deleting a volume attachment plan is reported in the watcher.
	harness.AddTest(func(c *tc.C) {
		s.deleteVolumeAttachmentPlan(c, vapChangeUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsChangeID),
		)
	})

	harness.Run(c, []string(nil))
}

// TestWatchMachineCloudInstance asserts the watcher behaviour for machine
// cloud instance changes through the service and state layers.
func (s *watcherSuite) TestWatchMachineCloudInstance(c *tc.C) {
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "storage"),
		loggertesting.WrapCheckLog(c),
	)

	machineUUID := s.newMachine(c)

	svc := service.NewService(state.NewState(s.TxnRunnerFactory()), factory)
	watcher, err := svc.WatchMachineCloudInstance(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(func(c *tc.C) {
		s.newMachineCloudInstance(c, machineUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(func(c *tc.C) {
		s.changeMachineCloudInstanceDisplayName(c, machineUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(func(c *tc.C) {
		s.deleteMachineCloudInstance(c, machineUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

// changeFilesystemAttachmentLife is a utility function for updating the life
// value of a filesystem attachment.
func (s *watcherSuite) changeFilesystemAttachmentLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_filesystem_attachment
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeFilesystemLife is a utility function for updating the life value of a
// filesystem.
func (s *watcherSuite) changeFilesystemLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_filesystem
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeFilesystemAttachmentMountPoint is a utility function for changing the
// mount point of a filesystem attachment. The purpose of this func is to
// change something about a filesystem attachment that isn't the life.
func (s *watcherSuite) changeFilesystemAttachmentMountPoint(
	c *tc.C, uuid string,
) {
	_, err := s.DB().Exec(`
UPDATE storage_filesystem_attachment
SET    mount_point = 'foobar'
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeFilesystemProviderID is a utility function for changing the provider id
// of a filesystem to a value choosen by this func. The purpose of this is to
// change something about a filesystem that isn't the life.
func (s *watcherSuite) changeFilesystemProviderID(
	c *tc.C, uuid string,
) {
	_, err := s.DB().Exec(`
UPDATE storage_filesystem
SET    provider_id = 'foobar'
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeMachineCloudInstanceDisplayName is a utility function for changing the
// display name of a machine cloud instance to a value choosen by this func. The
// purpose of this is to change something about a machine cloud instance to test
// watchers.
func (s *watcherSuite) changeMachineCloudInstanceDisplayName(
	c *tc.C, uuid string,
) {
	_, err := s.DB().Exec(`
UPDATE machine_cloud_instance
SET    display_name = 'foobar'
WHERE  machine_uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeProviderID is a utility function for changing the provider id
// of a filesystem to a value choosen by this func. The purpose of this is to
// change something about a filesystem that isn't the life.
func (s *watcherSuite) changeVolumeProviderID(
	c *tc.C, uuid string,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume
SET    provider_id = 'foobar'
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeAttachmentLife is a utility function for updating the life value
// of a volume attachment.
func (s *watcherSuite) changeVolumeAttchmentLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume_attachment
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeAttachmentPlanLife is a utility function for updating the life
// value of a volume attachment plan.
func (s *watcherSuite) changeVolumeAttchmentPlanLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume_attachment_plan
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeAttachmentReadOnly is a utility function for changing the mount
// point of a volume attachment. The purpose of this func is to change something
// about a volume attachment that isn't the life.
func (s *watcherSuite) changeVolumeAttachmentReadOnly(
	c *tc.C, uuid string,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume_attachment
SET    read_only = FALSE
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeLife is a utility function for updating the life value of a
// volume.
func (s *watcherSuite) changeVolumeLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// deleteFilesystem is a utility function for deleting a filesystem from the
// model.
func (s *watcherSuite) deleteFilesystem(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM storage_filesystem
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// deleteFilesystemAttachment is a utility function for deleting a filesystem
// attachment from the model.
func (s *watcherSuite) deleteFilesystemAttachment(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM storage_filesystem_attachment
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// deleteMachineCloudInstance deletes the cloud instance associated with the
// machine uuid.
func (s *watcherSuite) deleteMachineCloudInstance(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM machine_cloud_instance
WHERE  machine_uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// deleteVolume is a utility function for deleting a filesystem from the model.
func (s *watcherSuite) deleteVolume(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM storage_volume
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// deleteVolumeAttachment is a utility function for deleting a volume
// attachment from the model.
func (s *watcherSuite) deleteVolumeAttachment(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM storage_volume_attachment
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// deleteVolumeAttachmentPlan is a utility function for deleting a volume
// attachment plan from the model.
func (s *watcherSuite) deleteVolumeAttachmentPlan(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM storage_volume_attachment_plan
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// newMachineWithNetNode creates a new machine in the model attached to the
// supplied net node. The newly created machines uuid is returned along with the
// name.
func (s *watcherSuite) newMachine(c *tc.C) string {
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Check(err, tc.ErrorIsNil)
	machineUUID := machinetesting.GenUUID(c)
	name := "mfoo-" + machineUUID.String()

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO net_node VALUES (?)",
		netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO machine (uuid, name, net_node_uuid, life_id) VALUES (?, ?, ?, 0)",
		machineUUID.String(),
		name,
		netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return machineUUID.String()
}

// newMachineCloudInstance creates a new machine cloud instance in the model for
// the provided machine uuid.
func (s *watcherSuite) newMachineCloudInstance(
	c *tc.C, machineUUID string,
) {
	_, err := s.DB().ExecContext(
		c.Context(),
		"INSERT INTO machine_cloud_instance (machine_uuid, life_id) VALUES (?, 0)",
		machineUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
}

// newMachineFilesystem creates a new filesystem in the model with machine
// provision scope. Returned is the uuid and filesystem id of the entity.
func (s *watcherSuite) newMachineFilesystem(c *tc.C) (string, string) {
	fsUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	fsID := fmt.Sprintf("foo/%s", fsUUID.String())

	_, err = s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)
	`,
		fsUUID.String(), fsID)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID.String(), fsID
}

// newMachineFilesystemAttachmentForMachine creates a new filesystem attachment
// that has machine provision scope. The attachment is associated with the
// provided filesystem uuid and machine uuid.
func (s *watcherSuite) newMachineFilesystemAttachmentForMachine(
	c *tc.C, fsUUID string, machineUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	var netNodeUUID string
	err = s.DB().QueryRowContext(
		c.Context(),
		`
SELECT net_node_uuid
FROM machine
WHERE uuid = ?
		`,
		machineUUID,
	).Scan(&netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, 1)
`,
		attachmentUUID.String(), fsUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newModelFilesystem creates a new filesystem in the model with model
// provision scope. Returned is the uuid and filesystem id of the entity.
func (s *watcherSuite) newModelFilesystem(c *tc.C) (string, string) {
	fsUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	fsID := fmt.Sprintf("foo/%s", fsUUID.String())

	_, err = s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		fsUUID.String(), fsID)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID.String(), fsID
}

// newModelFilesystemAttachment creates a new filesystem attachment that has
// model provision scope. The attachment is associated with the provided
// filesystem uuid and machine uuid.
func (s *watcherSuite) newModelFilesystemAttachmentForMachine(
	c *tc.C, fsUUID string, machineUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	var netNodeUUID string
	err = s.DB().QueryRowContext(
		c.Context(),
		`
SELECT net_node_uuid
FROM machine
WHERE uuid = ?
		`,
		machineUUID,
	).Scan(&netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, 0)
`,
		attachmentUUID.String(), fsUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newMachineVolumeAttachmentForMachine creates a new volume attachment
// that has machine provision scope. The attachment is associated with the
// provided filesystem uuid and machine uuid.
func (s *watcherSuite) newMachineVolumeAttachmentForMachine(
	c *tc.C, fsUUID string, machineUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	var netNodeUUID string
	err = s.DB().QueryRowContext(
		c.Context(),
		`
SELECT net_node_uuid
FROM machine
WHERE uuid = ?
		`,
		machineUUID,
	).Scan(&netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume_attachment (uuid,
                                       storage_volume_uuid,
                                       net_node_uuid,
                                       life_id,
                                       provision_scope_id)
VALUES (?, ?, ?, 0, 1)
`,
		attachmentUUID.String(), fsUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newVolumeAttachmentForMachine creates a new volume attachment plan for the
// provided machine.
func (s *watcherSuite) newVolumeAttachmentPlanForMachine(
	c *tc.C, vsUUID string, machineUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	var netNodeUUID string
	err = s.DB().QueryRowContext(
		c.Context(),
		`
SELECT net_node_uuid
FROM machine
WHERE uuid = ?
		`,
		machineUUID,
	).Scan(&netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume_attachment_plan (uuid,
                                            storage_volume_uuid,
                                            net_node_uuid,
                                            life_id,
                                            provision_scope_id)
VALUES (?, ?, ?, 0, 1)
`,
		attachmentUUID.String(), vsUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newMachineVolume creates a new volume in the model with machine
// provision scope. Returned is the uuid and volume id of the entity.
func (s *watcherSuite) newMachineVolume(c *tc.C) (string, string) {
	vsUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	vsID := fmt.Sprintf("foo/%s", vsUUID.String())

	_, err = s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)
	`,
		vsUUID.String(), vsID)
	c.Assert(err, tc.ErrorIsNil)

	return vsUUID.String(), vsID
}

// newModelVolume creates a new volume in the model with model
// provision scope. Returned is the uuid and volume id of the entity.
func (s *watcherSuite) newModelVolume(c *tc.C) (string, string) {
	vsUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	vsID := fmt.Sprintf("foo/%s", vsUUID.String())

	_, err = s.DB().Exec(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		vsUUID.String(), vsID)
	c.Assert(err, tc.ErrorIsNil)

	return vsUUID.String(), vsID
}

// newModelVolumeAttachmentForMachine creates a new volume attachment
// that has model provision scope. The attachment is associated with the
// provided volume uuid and machine uuid.
func (s *watcherSuite) newModelVolumeAttachmentForMachine(
	c *tc.C, vsUUID string, machineUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	var netNodeUUID string
	err = s.DB().QueryRowContext(
		c.Context(),
		`
SELECT net_node_uuid
FROM machine
WHERE uuid = ?
		`,
		machineUUID,
	).Scan(&netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_volume_attachment (uuid,
                                       storage_volume_uuid,
                                       net_node_uuid,
                                       life_id,
                                       provision_scope_id)
VALUES (?, ?, ?, 0, 0)
`,
		attachmentUUID.String(), vsUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}
