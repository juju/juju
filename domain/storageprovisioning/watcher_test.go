// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	charmtesting "github.com/juju/juju/core/charm/testing"
	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	domainstorage "github.com/juju/juju/domain/storage"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/domain/storageprovisioning/service"
	"github.com/juju/juju/domain/storageprovisioning/state"
	domaintesting "github.com/juju/juju/domain/storageprovisioning/testing"
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

func (s *watcherSuite) setupService(c *tc.C) *service.Service {
	logger := loggertesting.WrapCheckLog(c)
	factory := domain.NewWatcherFactory(
		changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "storageprovisioning"),
		logger,
	)
	return service.NewService(state.NewState(s.TxnRunnerFactory()), factory, logger)
}

// TestWatchMachineProvisionedFilesystems asserts the watcher behaviour for
// machine provisioned filesystems through both the service and state layers.
func (s *watcherSuite) TestWatchMachineProvisionedFilesystems(c *tc.C) {
	svc := s.setupService(c)

	machineUUID := s.newMachine(c)
	watcher, err := svc.WatchMachineProvisionedFilesystems(c.Context(), coremachine.UUID(machineUUID))
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	var (
		fsOneUUID, fsTwoUUID storageprovisioning.FilesystemUUID
		fsOneID, fsTwoID     string
		fsaTwoUUID           string
	)

	// Assert that without any attachments machine provisioned filesystems do
	// not emit a change event on the watcher until at least one attachment
	// exists.
	harness.AddTest(c, func(c *tc.C) {
		fsOneUUID, fsOneID = s.newMachineFilesystem(c)
		fsTwoUUID, fsTwoID = s.newMachineFilesystem(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that adding the first attachment for the filesystem causes the
	// watcher to fire.
	harness.AddTest(c, func(c *tc.C) {
		s.newMachineFilesystemAttachmentForMachine(c, fsOneUUID.String(), machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsOneID),
		)
	})

	// Assert that adding the first attachment for the filesystem causes the
	// watcher to fire.
	harness.AddTest(c, func(c *tc.C) {
		fsaTwoUUID = s.newMachineFilesystemAttachmentForMachine(c, fsTwoUUID.String(), machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsTwoID),
		)
	})

	// Assert that a life change to a filesystem is reported by the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeFilesystemLife(c, fsOneUUID.String(), domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsOneID),
		)
	})

	// Assert that changing something about a filesystem which isn't the life
	// does not produce a change in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeFilesystemProviderID(c, fsTwoUUID.String())
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting the last filesystem attachment for a filesystem
	// results in the watcher firing.
	harness.AddTest(c, func(c *tc.C) {
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
	svc := s.setupService(c)

	machineUUID := s.newMachine(c)
	watcher, err := svc.WatchMachineProvisionedFilesystemAttachments(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	var fsaOneUUID, fsaTwoUUID string

	// Assert new machine provisioned filesystem attachments come out in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		fsUUID, _ := s.newModelFilesystem(c)
		fsaOneUUID = s.newMachineFilesystemAttachmentForMachine(c, fsUUID, machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsaOneUUID),
		)
	})

	// Assert new machine provisioned filesystem attachments come out in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		fsUUID, _ := s.newModelFilesystem(c)
		fsaTwoUUID = s.newMachineFilesystemAttachmentForMachine(c, fsUUID, machineUUID)

	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsaTwoUUID),
		)
	})

	// Assert that a life change to a filesystem attachment is reported in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeFilesystemAttachmentLife(c, fsaTwoUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsaTwoUUID),
		)
	})

	// Assert that changing something about a filesystem attachment which isn't
	// the life does not produce a change in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeFilesystemAttachmentMountPoint(c, fsaTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting a filesystem attachemtn is reported in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteFilesystemAttachment(c, fsaTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsaTwoUUID),
		)
	})

	harness.Run(c, []string(nil))
}

// TestWatchModelProvisionedFilesystemAttachments asserts the watcher behaviour
// for filesystem attachments that are model provisioned through both the
// service and state layers.
func (s *watcherSuite) TestWatchModelProvisionedFilesystemAttachments(c *tc.C) {
	svc := s.setupService(c)

	machineUUID := s.newMachine(c)
	watcher, err := svc.WatchModelProvisionedFilesystemAttachments(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	var fsaOneUUID, fsaTwoUUID string

	// Assert new machine provisioned filesystem attachments come out in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		fsUUID, _ := s.newModelFilesystem(c)
		fsaOneUUID = s.newModelFilesystemAttachmentForMachine(c, fsUUID, machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsaOneUUID),
		)
	})

	// Assert new machine provisioned filesystem attachments come out in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		fsUUID, _ := s.newModelFilesystem(c)
		fsaTwoUUID = s.newModelFilesystemAttachmentForMachine(c, fsUUID, machineUUID)

	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsaTwoUUID),
		)
	})

	// Assert that a life change to a filesystem attachment is reported in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeFilesystemAttachmentLife(c, fsaTwoUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsaTwoUUID),
		)
	})

	// Assert that changing something about a filesystem attachment which isn't
	// life or provider_id, does not produce a change in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeFilesystemAttachmentMountPoint(c, fsaTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that changing filesystem attachment provider id is reported in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeFilesystemAttachmentProviderID(c, fsaTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertChange()
	})

	// Assert that deleting a filesystem attachment is reported in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteFilesystemAttachment(c, fsaTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsaTwoUUID),
		)
	})

	harness.Run(c, []string{})
}

// TestWatchModelProvisionedFilesystems asserts the watcher behaviour for
// filesystems that are model provisioned through both the service and state
// layers.
func (s *watcherSuite) TestWatchModelProvisionedFilesystems(c *tc.C) {
	svc := s.setupService(c)

	watcher, err := svc.WatchModelProvisionedFilesystems(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	var fsOneID, fsTwoUUID, fsTwoID string

	// Assert new model provisioned filesystems come out in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		_, fsOneID = s.newModelFilesystem(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsOneID),
		)
	})

	// Assert new model provisioned filesystems come out in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		fsTwoUUID, fsTwoID = s.newModelFilesystem(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsTwoID),
		)
	})

	// Assert that a life change to a filesystem is reported in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeFilesystemLife(c, fsTwoUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsTwoID),
		)
	})

	// Assert that changing something about a filesystem which isn't the life or
	// provider_id, does not produce a change in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeFilesystemSizeMiB(c, fsTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that changing filesystem provider id is reported in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeFilesystemProviderID(c, fsTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertChange()
	})

	// Assert that deleting a filesystem is reported in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteFilesystem(c, fsTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(fsTwoID),
		)
	})

	harness.Run(c, []string{})
}

// TestWatchMachineProvisionedVolumes asserts the watcher behaviour for
// volumes that are machine provisioned through both the service and state
// layers.
func (s *watcherSuite) TestWatchMachineProvisionedVolumes(c *tc.C) {
	svc := s.setupService(c)

	machineUUID := s.newMachine(c)
	watcher, err := svc.WatchMachineProvisionedVolumes(c.Context(), coremachine.UUID(machineUUID))
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))
	var (
		vsOneUUID, vsTwoUUID storageprovisioning.VolumeUUID
		vsOneID, vsTwoID     string
		vsaTwoUUID           string
	)

	// Assert that without any attachments machine provisioned volumes do
	// not emit a change event on the watcher until at least one attachment
	// exists.
	harness.AddTest(c, func(c *tc.C) {
		vsOneUUID, vsOneID = s.newMachineVolume(c)
		vsTwoUUID, vsTwoID = s.newMachineVolume(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that adding the first attachment for the volume causes the
	// watcher to fire.
	harness.AddTest(c, func(c *tc.C) {
		s.newMachineVolumeAttachmentForMachine(c, vsOneUUID.String(), machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsOneID),
		)
	})

	// Assert that adding the first attachment for the volume causes the
	// watcher to fire.
	harness.AddTest(c, func(c *tc.C) {
		vsaTwoUUID = s.newMachineVolumeAttachmentForMachine(c, vsTwoUUID.String(), machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsTwoID),
		)
	})

	// Assert that a life change to a volume is reported by the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeVolumeLife(c, vsOneUUID.String(), domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsOneID),
		)
	})

	// Assert that changing something about a volume which isn't the life
	// does not produce a change in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeVolumeProviderID(c, vsTwoUUID.String())
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting the last volume attachment for a volume results in
	// the watcher firing.
	harness.AddTest(c, func(c *tc.C) {
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
	svc := s.setupService(c)

	machineUUID := s.newMachine(c)
	watcher, err := svc.WatchMachineProvisionedVolumeAttachments(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	var vsaOneUUID, vsaTwoUUID string

	// Assert new machine provisioned volume attachment comes out in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		vsUUID, _ := s.newMachineVolume(c)
		vsaOneUUID = s.newMachineVolumeAttachmentForMachine(c, vsUUID.String(), machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsaOneUUID),
		)
	})

	// Assert new machine provisioned volume attachment comes out in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		vsUUID, _ := s.newMachineVolume(c)
		vsaTwoUUID = s.newMachineVolumeAttachmentForMachine(c, vsUUID.String(), machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsaTwoUUID),
		)
	})

	// Assert that a life change to a volume attachment is reported in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeVolumeAttchmentLife(c, vsaTwoUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsaTwoUUID),
		)
	})

	// Assert that changing something about a volume attachment which isn't
	// the life does not produce a change in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeVolumeAttachmentReadOnly(c, vsaTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting a volume attachment is reported in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteVolumeAttachment(c, vsaTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsaTwoUUID),
		)
	})

	harness.Run(c, []string(nil))
}

// TestWatchModelProvisionedVolumeAttachments asserts the watcher behaviour
// for volume attachments that are model provisioned through both the service
// and state layers.
func (s *watcherSuite) TestWatchModelProvisionedVolumeAttachments(c *tc.C) {
	svc := s.setupService(c)

	machineUUID := s.newMachine(c)
	watcher, err := svc.WatchModelProvisionedVolumeAttachments(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	var vsaOneUUID, vsaTwoUUID string

	// Assert new machine provisioned volume attachment comes out in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		vsUUID, _ := s.newModelVolume(c)
		vsaOneUUID = s.newModelVolumeAttachmentForMachine(c, vsUUID, machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsaOneUUID),
		)
	})

	// Assert new machine provisioned volume attachment comes out in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		vsUUID, _ := s.newModelVolume(c)
		vsaTwoUUID = s.newModelVolumeAttachmentForMachine(c, vsUUID, machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsaTwoUUID),
		)
	})

	// Assert that a life change to a volume attachment is reported in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeVolumeAttchmentLife(c, vsaTwoUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsaTwoUUID),
		)
	})

	// Assert that changing something about a volume attachment which isn't
	// the life does not produce a change in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeVolumeAttachmentReadOnly(c, vsaTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting a volume attachment is reported in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteVolumeAttachment(c, vsaTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsaTwoUUID),
		)
	})

	harness.Run(c, []string{})
}

// TestWatchModelProvisionedVolumes asserts the watcher behaviour for
// volumes that are model provisioned through both the service and state layers.
func (s *watcherSuite) TestWatchModelProvisionedVolumes(c *tc.C) {
	svc := s.setupService(c)

	watcher, err := svc.WatchModelProvisionedVolumes(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	var vOneID, vTwoID, vTwoUUID string

	// Assert new model provisioned volumes come out in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		_, vOneID = s.newModelVolume(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vOneID),
		)
	})

	// Assert new model provisioned volumes come out in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		vTwoUUID, vTwoID = s.newModelVolume(c)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vTwoID),
		)
	})

	// Assert that a life change to a volume is reported in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeVolumeLife(c, vTwoUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vTwoID),
		)
	})

	// Assert that changing something about a volume which isn't the life
	// does not produce a change in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeVolumeProviderID(c, vTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// Assert that deleting a volume is reported in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteVolume(c, vTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vTwoID),
		)
	})

	harness.Run(c, []string{})
}

// TestWatchVolumeAttachmentPlans asserts the watcher behaviour for volume
// attachment plans through both the service and state layers.
func (s *watcherSuite) TestWatchVolumeAttachmentPlans(c *tc.C) {
	svc := s.setupService(c)

	machineUUID := s.newMachine(c)
	watcher, err := svc.WatchVolumeAttachmentPlans(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	var vsOneID, vapTwoUUID, vsTwoID string

	// Assert new volume attachment plans come out in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		var vUUID storageprovisioning.VolumeUUID
		vUUID, vsOneID = s.newMachineVolume(c)
		s.newVolumeAttachmentPlanForMachine(c, vUUID.String(), machineUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			// We expect that the volume attchment plan watcher outputs volume ids.
			watchertest.StringSliceAssert(vsOneID),
		)
	})

	// Assert new volume attachment plans come out in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		var vUUID storageprovisioning.VolumeUUID
		vUUID, vsTwoID = s.newMachineVolume(c)
		vapTwoUUID = s.newVolumeAttachmentPlanForMachine(c, vUUID.String(), machineUUID)

	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			// We expect that the volume attchment plan watcher outputs volume ids.
			watchertest.StringSliceAssert(vsTwoID),
		)
	})

	// Assert that a life change to a volume attachment plan is reported in the
	// watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.changeVolumeAttchmentPlanLife(c, vapTwoUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsTwoID),
		)
	})

	// Assert that deleting a volume attachment plan is reported in the watcher.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteVolumeAttachmentPlan(c, vapTwoUUID)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(vsTwoID),
		)
	})

	harness.Run(c, []string(nil))
}

// TestWatchMachineCloudInstance asserts the watcher behaviour for machine
// cloud instance changes through the service and state layers.
func (s *watcherSuite) TestWatchMachineCloudInstance(c *tc.C) {
	svc := s.setupService(c)

	machineUUID := s.newMachine(c)
	watcher, err := svc.WatchMachineCloudInstance(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
		s.newMachineCloudInstance(c, machineUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		s.changeMachineCloudInstanceDisplayName(c, machineUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.AddTest(c, func(c *tc.C) {
		s.deleteMachineCloudInstance(c, machineUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

// TestWatchStorageAttachmentsForUnit is testing that for any life change made
// to a unit storage attachment the watcher's client is informed of the
// corresponding storage ids of the attachments that changed.
func (s *watcherSuite) TestWatchStorageAttachmentsForUnit(c *tc.C) {
	svc := s.setupService(c)

	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, _, _ := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)

	storageInstanceUUID1, storageID1 := s.newStorageInstance(c)
	storageInstanceUUID2, storageID2 := s.newStorageInstance(c)
	storageInstanceUUID3, storageID3 := s.newStorageInstance(c)

	watcher, err := svc.WatchStorageAttachmentsForUnit(
		c.Context(), unitUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Attach the three storage instances to the unit. This MUST create three
	// events in the watcher each containing the id of the storage instance.
	var (
		storageAttachmentUUID1 storageprovisioning.StorageAttachmentUUID
		storageAttachmentUUID2 storageprovisioning.StorageAttachmentUUID
		storageAttachmentUUID3 storageprovisioning.StorageAttachmentUUID
	)
	harness.AddTest(c, func(c *tc.C) {
		attachmenUUIDs := s.newStorageAttachmentsForInstances(
			c,
			unitUUID,
			storageInstanceUUID1,
			storageInstanceUUID2,
			storageInstanceUUID3,
		)

		storageAttachmentUUID1 = attachmenUUIDs[0]
		storageAttachmentUUID2 = attachmenUUIDs[1]
		storageAttachmentUUID3 = attachmenUUIDs[2]
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				storageID1,
				storageID2,
				storageID3,
			),
		)
	})

	// Set attachments 1 and 2 to dying. This must trigger two events on the
	// watcher one for each storage instance id the attachment corresponds to.
	harness.AddTest(c, func(c *tc.C) {
		s.changeStorageAttachmentLives(
			c,
			map[storageprovisioning.StorageAttachmentUUID]domainlife.Life{
				storageAttachmentUUID1: domainlife.Dying,
				storageAttachmentUUID2: domainlife.Dying,
			},
		)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				storageID1,
				storageID2,
			),
		)
	})

	// Set attachments 1 to dead and 3 to dying. This must trigger two events on
	// watcher one for each storage instance id the attachment corresponds to.
	harness.AddTest(c, func(c *tc.C) {
		s.changeStorageAttachmentLives(
			c,
			map[storageprovisioning.StorageAttachmentUUID]domainlife.Life{
				storageAttachmentUUID1: domainlife.Dead,
				storageAttachmentUUID3: domainlife.Dying,
			},
		)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(
			watchertest.StringSliceAssert(
				storageID1,
				storageID3,
			),
		)
	})

	harness.Run(c, []string(nil))
}

func (s *watcherSuite) TestWatchStorageAttachmentForVolume(c *tc.C) {
	svc := s.setupService(c)

	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, _, netNodeUUID := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	storageInstanceUUID, _ := s.newStorageInstance(c)
	storageAttachmentUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID, domainlife.Alive)
	volumeUUID, _ := s.newMachineVolume(c)

	watcher, err := svc.WatchStorageAttachment(
		c.Context(), storageAttachmentUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// storage instance volume creation should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		s.newStorageInstanceVolume(c, storageInstanceUUID, volumeUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	var blockDeviceUUID string
	var machineUUID string
	harness.AddTest(c, func(c *tc.C) {
		machineUUID = s.newMachineWithNetNode(c, netNodeUUID.String())
		blockDeviceUUID = s.newBlockDevice(c, machineUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	var vaUUID string
	// storage volume attachment creation should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		vaUUID = s.newModelVolumeAttachmentForNetNode(c, volumeUUID.String(), netNodeUUID.String())
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// storage volume attachment update should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		s.changeVolumeAttachmentLife(c, vaUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// storage volume attachment update should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		s.setVolumeAttachmentBlockDevice(c, vaUUID, blockDeviceUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// block device update should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		s.changeBlockDeviceMountPoint(c, blockDeviceUUID, "/mnt/foo")
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// block device link device creation should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		s.newBlockDeviceLinkDevice(c, blockDeviceUUID, machineUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// block device link device update should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		s.renameBlockDeviceLinkDevice(c, blockDeviceUUID, "foo")
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// block device link device deletion should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteBlockDeviceLinkDevice(c, blockDeviceUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// storage volume attachment deletion should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteVolumeAttachment(c, vaUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

func (s *watcherSuite) TestWatchStorageAttachmentForFilesystem(c *tc.C) {
	svc := s.setupService(c)

	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, _, netNodeUUID := s.newUnitWithNetNode(c, "foo/0", appUUID, charmUUID)
	storageInstanceUUID, _ := s.newStorageInstance(c)
	storageAttachmentUUID := s.newStorageAttachment(c, storageInstanceUUID, unitUUID, domainlife.Alive)
	fsUUID, _ := s.newMachineFilesystem(c)

	watcher, err := svc.WatchStorageAttachment(
		c.Context(), storageAttachmentUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// storage instance filesystem creation should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		s.newStorageInstanceFilesystem(c, storageInstanceUUID, fsUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	var fsaUUID string
	// storage filesystem attachment creation should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		fsaUUID = s.newModelFilesystemAttachmentForNetNode(c, fsUUID.String(), netNodeUUID.String())
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// storage filesystem attachment update should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		s.changeFilesystemAttachmentLife(c, fsaUUID, domainlife.Dying)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	// storage filesystem attachment deletion should trigger a change.
	harness.AddTest(c, func(c *tc.C) {
		s.deleteFilesystemAttachment(c, fsaUUID)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertChange()
	})

	harness.Run(c, struct{}{})
}

// changeStorageAttachmentLives is a utility function for updating the life
// value of zero or more storage attachment uuids. This function is designed
// to perform all of the changes within a single transaction.
func (s *watcherSuite) changeStorageAttachmentLives(
	c *tc.C, changes map[storageprovisioning.StorageAttachmentUUID]domainlife.Life,
) {
	if len(changes) == 0 {
		return
	}

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		for uuid, life := range changes {
			_, err := tx.ExecContext(
				ctx,
				`
UPDATE storage_attachment
SET    life_id = ?
WHERE  uuid = ?
`,
				int(life),
				uuid.String(),
			)
			if err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

// changeFilesystemAttachmentLife is a utility function for updating the life
// value of a filesystem attachment.
func (s *watcherSuite) changeFilesystemAttachmentLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
UPDATE storage_filesystem_attachment
SET    life_id = ?
WHERE  uuid = ?
`,
			int(life), uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) changeVolumeAttachmentLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE storage_volume_attachment
SET    life_id = ?
WHERE  uuid = ?`, int(life), uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// changeFilesystemLife is a utility function for updating the life value of a
// filesystem.
func (s *watcherSuite) changeFilesystemLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
UPDATE storage_filesystem
SET    life_id = ?
WHERE  uuid = ?
`,
			int(life), uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// changeFilesystemAttachmentMountPoint is a utility function for changing the
// mount point of a filesystem attachment. The purpose of this func is to
// change something about a filesystem attachment that isn't the life.
func (s *watcherSuite) changeFilesystemAttachmentMountPoint(
	c *tc.C, uuid string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE storage_filesystem_attachment
SET    mount_point = 'foobar'
WHERE  uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// changeFilesystemProviderID is a utility function for changing the provider id
// of a filesystem to a value chosen by this func. The purpose of this is to
// help test chaning the provider id, which the watcher is interested in.
func (s *watcherSuite) changeFilesystemProviderID(
	c *tc.C, uuid string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
UPDATE storage_filesystem
SET    provider_id = 'foobar'
WHERE  uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// changeFilesystemSizeMiB is a utility function for changing the size_mib of a
// filesystem to a value chosen by this func. The purpose of this is to change
// something about a filesystem that isn't the life or provider_id.
func (s *watcherSuite) changeFilesystemSizeMiB(
	c *tc.C, uuid string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
UPDATE storage_filesystem
SET    size_mib = 9001
WHERE  uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// changeFilesystemAttachmentProviderID is a utility function for changing the
// provider id of a filesystem attachment to a value chosen by this func. The
// purpose of this is to help test chaning the provider id, which the watcher is
// interested in.
func (s *watcherSuite) changeFilesystemAttachmentProviderID(
	c *tc.C, uuid string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
UPDATE storage_filesystem_attachment
SET    provider_id = 'foobar'
WHERE  uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// changeMachineCloudInstanceDisplayName is a utility function for changing the
// display name of a machine cloud instance to a value chosen by this func. The
// purpose of this is to change something about a machine cloud instance to test
// watchers.
func (s *watcherSuite) changeMachineCloudInstanceDisplayName(
	c *tc.C, uuid string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
UPDATE machine_cloud_instance
SET    display_name = 'foobar'
WHERE  machine_uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeProviderID is a utility function for changing the provider id
// of a filesystem to a value chosen by this func. The purpose of this is to
// change something about a filesystem that isn't the life.
func (s *watcherSuite) changeVolumeProviderID(
	c *tc.C, uuid string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
UPDATE storage_volume
SET    provider_id = 'foobar'
WHERE  uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeAttachmentLife is a utility function for updating the life value
// of a volume attachment.
func (s *watcherSuite) changeVolumeAttchmentLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
UPDATE storage_volume_attachment
SET    life_id = ?
WHERE  uuid = ?
`,
			int(life), uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeAttachmentPlanLife is a utility function for updating the life
// value of a volume attachment plan.
func (s *watcherSuite) changeVolumeAttchmentPlanLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
UPDATE storage_volume_attachment_plan
SET    life_id = ?
WHERE  uuid = ?
`,
			int(life),
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeAttachmentReadOnly is a utility function for changing the mount
// point of a volume attachment. The purpose of this func is to change something
// about a volume attachment that isn't the life.
func (s *watcherSuite) changeVolumeAttachmentReadOnly(
	c *tc.C, uuid string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
UPDATE storage_volume_attachment
SET    read_only = FALSE
WHERE  uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeLife is a utility function for updating the life value of a
// volume.
func (s *watcherSuite) changeVolumeLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
UPDATE storage_volume
SET    life_id = ?
WHERE  uuid = ?
`,
			int(life),
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// deleteFilesystem is a utility function for deleting a filesystem from the
// model.
func (s *watcherSuite) deleteFilesystem(c *tc.C, uuid string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
DELETE FROM storage_filesystem
WHERE  uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// deleteFilesystemAttachment is a utility function for deleting a filesystem
// attachment from the model.
func (s *watcherSuite) deleteFilesystemAttachment(c *tc.C, uuid string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
DELETE FROM storage_filesystem_attachment
WHERE  uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// deleteMachineCloudInstance deletes the cloud instance associated with the
// machine uuid.
func (s *watcherSuite) deleteMachineCloudInstance(c *tc.C, uuid string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
DELETE FROM machine_cloud_instance
WHERE  machine_uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// deleteVolume is a utility function for deleting a filesystem from the model.
func (s *watcherSuite) deleteVolume(c *tc.C, uuid string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
DELETE FROM storage_volume
WHERE  uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// deleteVolumeAttachment is a utility function for deleting a volume
// attachment from the model.
func (s *watcherSuite) deleteVolumeAttachment(c *tc.C, uuid string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
DELETE FROM storage_volume_attachment
WHERE  uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// deleteVolumeAttachmentPlan is a utility function for deleting a volume
// attachment plan from the model.
func (s *watcherSuite) deleteVolumeAttachmentPlan(c *tc.C, uuid string) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
DELETE FROM storage_volume_attachment_plan
WHERE  uuid = ?
`,
			uuid,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// newMachineWithNetNode creates a new machine in the model attached to the
// supplied net node. The newly created machines uuid is returned along with the
// name.
func (s *watcherSuite) newMachine(c *tc.C) string {
	machineUUID := machinetesting.GenUUID(c)
	name := "mfoo-" + machineUUID.String()

	nodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO net_node (uuid) VALUES (?)`, nodeUUID.String())
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
INSERT INTO machine (uuid, name, net_node_uuid, life_id) VALUES (?, ?, ?, 0)`,
			machineUUID.String(),
			name,
			nodeUUID.String(),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return machineUUID.String()
}

func (s *watcherSuite) newMachineWithNetNode(c *tc.C, netNodeUUID string) string {
	machineUUID := machinetesting.GenUUID(c)
	name := "mfoo-" + machineUUID.String()

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {

		_, err := tx.Exec(`
INSERT INTO machine (uuid, name, net_node_uuid, life_id) VALUES (?, ?, ?, 0)`,
			machineUUID.String(),
			name,
			netNodeUUID,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return machineUUID.String()
}

// newMachineCloudInstance creates a new machine cloud instance in the model for
// the provided machine uuid.
func (s *watcherSuite) newMachineCloudInstance(
	c *tc.C, machineUUID string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			"INSERT INTO machine_cloud_instance (machine_uuid, life_id) VALUES (?, 0)",
			machineUUID,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// newMachineFilesystem creates a new filesystem in the model with machine
// provision scope. Returned is the uuid and filesystem id of the entity.
func (s *watcherSuite) newMachineFilesystem(c *tc.C) (storageprovisioning.FilesystemUUID, string) {
	fsUUID := domaintesting.GenFilesystemUUID(c)

	fsID := fmt.Sprintf("foo/%s", fsUUID.String())

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx, `
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)
		`,
			fsUUID.String(),
			fsID,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID, fsID
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
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err = tx.QueryRowContext(
			ctx,
			`
SELECT net_node_uuid
FROM machine
WHERE uuid = ?
			`,
			machineUUID,
		).Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, 1)
`,
			attachmentUUID.String(), fsUUID, netNodeUUID,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newModelFilesystem creates a new filesystem in the model with model
// provision scope. Returned is the uuid and filesystem id of the entity.
func (s *watcherSuite) newModelFilesystem(c *tc.C) (string, string) {
	fsUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	fsID := fmt.Sprintf("foo/%s", fsUUID.String())

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
		`,
			fsUUID.String(),
			fsID,
		)
		return err
	})
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
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(
			ctx,
			`
SELECT net_node_uuid
FROM machine
WHERE uuid = ?
			`,
			machineUUID,
		).Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, 0)
`,
			attachmentUUID.String(),
			fsUUID,
			netNodeUUID,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

func (s *watcherSuite) newModelFilesystemAttachmentForNetNode(
	c *tc.C, fsUUID string, netNodeUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, 0)
`,
			attachmentUUID.String(),
			fsUUID,
			netNodeUUID,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newMachineVolumeAttachmentForMachine creates a new volume attachment
// that has machine provision scope. The attachment is associated with the
// provided filesystem uuid and machine uuid.
func (s *watcherSuite) newMachineVolumeAttachmentForMachine(
	c *tc.C, volUUID string, machineUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	var netNodeUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(
			ctx,
			`
SELECT net_node_uuid
FROM machine
WHERE uuid = ?
			`,
			machineUUID,
		).Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			`
INSERT INTO storage_volume_attachment (uuid,
                                       storage_volume_uuid,
                                       net_node_uuid,
                                       life_id,
                                       provision_scope_id)
VALUES (?, ?, ?, 0, 1)
`,
			attachmentUUID.String(), volUUID, netNodeUUID,
		)
		return err
	})
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
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(
			ctx,
			`
SELECT net_node_uuid
FROM machine
WHERE uuid = ?
			`,
			machineUUID,
		).Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(
			ctx,
			`
INSERT INTO storage_volume_attachment_plan (uuid,
                                            storage_volume_uuid,
                                            net_node_uuid,
                                            life_id,
                                            provision_scope_id)
VALUES (?, ?, ?, 0, 1)
`,
			attachmentUUID.String(), vsUUID, netNodeUUID,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newMachineVolume creates a new volume in the model with machine
// provision scope. Returned is the uuid and volume id of the entity.
func (s *watcherSuite) newMachineVolume(c *tc.C) (storageprovisioning.VolumeUUID, string) {
	vsUUID := domaintesting.GenVolumeUUID(c)

	vsID := fmt.Sprintf("foo/%s", vsUUID.String())

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)
		`,
			vsUUID.String(),
			vsID,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return vsUUID, vsID
}

// newModelVolume creates a new volume in the model with model
// provision scope. Returned is the uuid and volume id of the entity.
func (s *watcherSuite) newModelVolume(c *tc.C) (string, string) {
	vsUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	vsID := fmt.Sprintf("foo/%s", vsUUID.String())
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
		`,
			vsUUID.String(),
			vsID,
		)
		return err
	})
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
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT net_node_uuid
FROM machine
WHERE uuid = ?`, machineUUID).Scan(&netNodeUUID)
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
INSERT INTO storage_volume_attachment (
    uuid,
    storage_volume_uuid,
    net_node_uuid,
    life_id,
    provision_scope_id)
VALUES (?, ?, ?, 0, 0)`, attachmentUUID.String(), vsUUID, netNodeUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

func (s *watcherSuite) newModelVolumeAttachmentForNetNode(
	c *tc.C, vsUUID string, netNodeUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_volume_attachment (
    uuid,
    storage_volume_uuid,
    net_node_uuid,
    life_id,
    provision_scope_id)
VALUES (?, ?, ?, 0, 0)`, attachmentUUID.String(), vsUUID, netNodeUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

func (s *watcherSuite) newApplication(c *tc.C, name string) (string, string) {
	appUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	charmUUID := charmtesting.GenCharmID(c)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)`, charmUUID.String(), "foo")
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, 'myapp')`, charmUUID.String())
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)`, appUUID.String(), charmUUID, name, network.AlphaSpaceId)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return appUUID.String(), charmUUID.String()
}

func (s *watcherSuite) newUnitWithNetNode(
	c *tc.C, name, appUUID, charmUUID string,
) (coreunit.UUID, coreunit.Name, domainnetwork.NetNodeUUID) {
	unitUUID := unittesting.GenUnitUUID(c)
	nodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err = tx.ExecContext(ctx, `
INSERT INTO net_node (uuid) VALUES (?)`, nodeUUID.String())
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)`,
			unitUUID.String(), name, appUUID, charmUUID, nodeUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID, coreunit.Name(name), nodeUUID
}

type preparer struct{}

func (p preparer) Prepare(query string, typeSamples ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, typeSamples...)
}

func (s *watcherSuite) nextStorageSequenceNumber(
	c *tc.C,
) uint64 {
	var id uint64
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		id, err = sequencestate.NextValue(
			ctx, preparer{}, tx, domainsequence.StaticNamespace("storage"),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return id
}

func (s *watcherSuite) newStoragePool(c *tc.C, name string, providerType string, attrs map[string]string) string {
	spUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_pool (uuid, name, type)
VALUES (?, ?, ?)`, spUUID.String(), name, providerType)
		if err != nil {
			return err
		}

		for k, v := range attrs {
			_, err = tx.ExecContext(ctx, `
INSERT INTO storage_pool_attribute (storage_pool_uuid, key, value)
VALUES (?, ?, ?)`, spUUID.String(), k, v)
			if err != nil {
				return err
			}
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return spUUID.String()
}

func (s *watcherSuite) newStorageInstance(c *tc.C) (
	domainstorage.StorageInstanceUUID, string,
) {
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)
	seq := s.nextStorageSequenceNumber(c)
	storageName := fmt.Sprintf("mystorage-%d", seq)
	storageID := fmt.Sprintf("mystorage/%d", seq)
	poolUUID := s.newStoragePool(c, storageInstanceUUID.String(), "test-provider", nil)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_instance(uuid, charm_name, storage_name, storage_id,
                             storage_kind_id, life_id, requested_size_mib,
                             storage_pool_uuid)
VALUES (?, ?, ?, ?, 1, 0, 100, ?)`,
			storageInstanceUUID.String(),
			"my-charm",
			storageName,
			storageID,
			poolUUID,
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return storageInstanceUUID, storageID
}

func (s *watcherSuite) newStorageAttachment(
	c *tc.C,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
	unitUUID coreunit.UUID,
	life domainlife.Life,
) storageprovisioning.StorageAttachmentUUID {
	saUUID := domaintesting.GenStorageAttachmentUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, ?, ?)`,
			saUUID.String(), storageInstanceUUID.String(), unitUUID.String(), life)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return saUUID
}

// newStorageAttachmentsForInstances is responsible for create storage
// attachments on to each of the supplied storage instances using the unit uuid.
// Each attachment created sets a default life of alive.
//
// Use this function when it is required that all of the storage attachments be
// created within a single transaction.
func (s *watcherSuite) newStorageAttachmentsForInstances(
	c *tc.C,
	unitUUID coreunit.UUID,
	instances ...domainstorage.StorageInstanceUUID,
) []storageprovisioning.StorageAttachmentUUID {
	rval := make([]storageprovisioning.StorageAttachmentUUID, 0, len(instances))

	err := s.TxnRunner().StdTxn(
		c.Context(),
		func(ctx context.Context, tx *sql.Tx) error {
			for _, instUUID := range instances {
				attachmentUUID, err := storageprovisioning.NewStorageAttachmentUUID()
				if err != nil {
					return err
				}
				rval = append(rval, attachmentUUID)
				_, err = tx.ExecContext(
					ctx,
					`
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, ?, ?)
`,
					attachmentUUID.String(),
					instUUID.String(),
					unitUUID.String(),
					domainlife.Alive,
				)
				if err != nil {
					return err
				}
			}
			return nil
		},
	)

	c.Assert(err, tc.ErrorIsNil)

	return rval
}

func (s *watcherSuite) newStorageInstanceFilesystem(
	c *tc.C, instanceUUID domainstorage.StorageInstanceUUID,
	filesystemUUID storageprovisioning.FilesystemUUID,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_instance_filesystem (storage_instance_uuid, storage_filesystem_uuid)
VALUES (?, ?)`, instanceUUID.String(), filesystemUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) newStorageInstanceVolume(
	c *tc.C, instanceUUID domainstorage.StorageInstanceUUID,
	volumeUUID storageprovisioning.VolumeUUID,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)`, instanceUUID.String(), volumeUUID.String())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) newBlockDevice(
	c *tc.C,
	machineUUID string,
) string {
	blockDeviceUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err = tx.ExecContext(ctx, `
INSERT INTO block_device (uuid, name, machine_uuid)
VALUES (?, ?, ?)`, blockDeviceUUID.String(), blockDeviceUUID.String(), machineUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return blockDeviceUUID.String()
}

func (s *watcherSuite) setVolumeAttachmentBlockDevice(
	c *tc.C, attachmentUUID string, blockDeviceUUID string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE storage_volume_attachment
SET    block_device_uuid = ?
WHERE  uuid = ?`, blockDeviceUUID, attachmentUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) newBlockDeviceLinkDevice(
	c *tc.C,
	blockDeviceUUID string,
	machineUUID string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO block_device_link_device (block_device_uuid, machine_uuid, name)
VALUES (?, ?, ?)`, blockDeviceUUID, machineUUID, blockDeviceUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) changeBlockDeviceMountPoint(
	c *tc.C, blockDeviceUUID string, mountPoint string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE block_device
SET    mount_point = ?
WHERE  uuid = ?`, mountPoint, blockDeviceUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) renameBlockDeviceLinkDevice(
	c *tc.C,
	blockDeviceUUID string,
	newName string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
UPDATE block_device_link_device
SET name = ?
WHERE block_device_uuid = ?`, newName, blockDeviceUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *watcherSuite) deleteBlockDeviceLinkDevice(
	c *tc.C,
	blockDeviceUUID string,
) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
DELETE FROM block_device_link_device
WHERE block_device_uuid = ?`, blockDeviceUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}
