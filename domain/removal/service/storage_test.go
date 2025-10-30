// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	storageprovtesting "github.com/juju/juju/domain/storageprovisioning/testing"
	"github.com/juju/juju/internal/errors"
)

type storageSuite struct {
	baseSuite
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestRemoveStorageAttachmentNoForceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := storageprovtesting.GenStorageAttachmentUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.StorageAttachmentExists(gomock.Any(), saUUID.String()).Return(true, nil)
	exp.EnsureStorageAttachmentNotAlive(gomock.Any(), saUUID.String()).Return(nil)
	exp.StorageAttachmentScheduleRemoval(gomock.Any(), gomock.Any(), saUUID.String(), false, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveStorageAttachment(c.Context(), saUUID, false, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *storageSuite) TestRemoveStorageAttachmentForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := storageprovtesting.GenStorageAttachmentUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	exp := s.modelState.EXPECT()
	exp.StorageAttachmentExists(gomock.Any(), saUUID.String()).Return(true, nil)
	exp.EnsureStorageAttachmentNotAlive(gomock.Any(), saUUID.String()).Return(nil)
	exp.StorageAttachmentScheduleRemoval(gomock.Any(), gomock.Any(), saUUID.String(), true, when.UTC()).Return(nil)

	jobUUID, err := s.newService(c).RemoveStorageAttachment(c.Context(), saUUID, true, 0)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *storageSuite) TestRemoveStorageAttachmentForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := storageprovtesting.GenStorageAttachmentUUID(c)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	exp := s.modelState.EXPECT()
	exp.StorageAttachmentExists(gomock.Any(), saUUID.String()).Return(true, nil)
	exp.EnsureStorageAttachmentNotAlive(gomock.Any(), saUUID.String()).Return(nil)

	// The first normal removal scheduled immediately.
	exp.StorageAttachmentScheduleRemoval(gomock.Any(), gomock.Any(), saUUID.String(), false, when.UTC()).Return(nil)

	// The forced removal scheduled after the wait duration.
	exp.StorageAttachmentScheduleRemoval(gomock.Any(), gomock.Any(), saUUID.String(), true, when.UTC().Add(time.Minute)).Return(nil)

	jobUUID, err := s.newService(c).RemoveStorageAttachment(c.Context(), saUUID, true, time.Minute)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

// TestRemoveStorageAttachmentFromAliveUnitNotFound tests that requesting to
// remove a storage attachment that doesn't exists results in a
// [storageerrors.StorageAttachmentNotFound] error to the caller.
func (s *storageSuite) TestRemoveStorageAttachmentFromAliveUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)

	exp := s.modelState.EXPECT()
	exp.GetDetachInfoForStorageAttachment(
		gomock.Any(), saUUID.String(),
	).Return(
		internal.StorageAttachmentDetachInfo{},
		storageerrors.StorageAttachmentNotFound,
	)

	_, err := s.newService(c).RemoveStorageAttachmentFromAliveUnit(
		c.Context(),
		saUUID,
		false,
		0,
	)
	c.Check(err, tc.ErrorIs, storageerrors.StorageAttachmentNotFound)
}

// TestRemoveStorageAttachmentFromAliveUnitNotFound tests that requesting to
// remove at least one storage attachment that doesn't exists fails the whole
// operation. We don't want to any other storage attachments removed and the
// caller get back an error which satisfies
// [storageerrors.StorageAttachmentNotFound].
func (s *storageSuite) TestRemoveStorageAttachmentFromAliveUnitUnitNotAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	detatchInfo := internal.StorageAttachmentDetachInfo{
		CharmStorageName: "kratos-key-store",
		CountFulfilment:  2,
		RequiredCountMin: 1,
		Life:             int(life.Alive),
		UnitLife:         int(life.Dying),
		UnitUUID:         unitUUID.String(),
	}
	exp := s.modelState.EXPECT()
	exp.GetDetachInfoForStorageAttachment(
		gomock.Any(), saUUID.String(),
	).Return(detatchInfo, nil)

	_, err := s.newService(c).RemoveStorageAttachmentFromAliveUnit(
		c.Context(),
		saUUID,
		false,
		0,
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotAlive)
}

// TestRemoveStorageAttachmentFromAliveUnitMinViolation tests that removing a
// storage attachment which would violate the charms minimum storage
// requirements results in a [applicationerrors.UnitStorageMinViolation] error.
func (s *storageSuite) TestRemoveStorageAttachmentFromAliveUnitMinViolation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	detatchInfo := internal.StorageAttachmentDetachInfo{
		CharmStorageName: "kratos-key-store",
		CountFulfilment:  2,
		RequiredCountMin: 2,
		Life:             int(life.Alive),
		UnitLife:         int(life.Alive),
		UnitUUID:         unitUUID.String(),
	}
	exp := s.modelState.EXPECT()
	exp.GetDetachInfoForStorageAttachment(
		gomock.Any(), saUUID.String(),
	).Return(detatchInfo, nil)

	_, err := s.newService(c).RemoveStorageAttachmentFromAliveUnit(
		c.Context(),
		saUUID,
		false,
		0,
	)

	storageErr, has := errors.AsType[applicationerrors.UnitStorageMinViolation](err)
	c.Check(has, tc.IsTrue)
	c.Check(storageErr, tc.Equals, applicationerrors.UnitStorageMinViolation{
		CharmStorageName: "kratos-key-store",
		RequiredMinimum:  2,
		UnitUUID:         unitUUID.String(),
	})
}

// TestRemoveStorageAttachmentFromAliveUnitFulfilmentError tests that when
// ensuring a storage attachment is not alive but the fulfilment condition fails
// [Service.RemoveStorageAttachmentFromAliveUnit] returns to the caller a
// [applicationerrors.UnitStorageMinViolation] error.
//
// We would expect to see this type of situation when the unit's storage changes
// after the service has calculated their assumptions.
func (s *storageSuite) TestRemoveStorageAttachmentFromAliveUnitFulfilmentError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	detatchInfo := internal.StorageAttachmentDetachInfo{
		CharmStorageName: "kratos-key-store",
		CountFulfilment:  3,
		RequiredCountMin: 2,
		Life:             int(life.Alive),
		UnitLife:         int(life.Alive),
		UnitUUID:         unitUUID.String(),
	}
	exp := s.modelState.EXPECT()
	exp.GetDetachInfoForStorageAttachment(
		gomock.Any(), saUUID.String(),
	).Return(detatchInfo, nil)
	exp.EnsureStorageAttachmentNotAliveWithFulfilment(
		gomock.Any(), saUUID.String(), 2,
	).Return(removalerrors.StorageFulfilmentNotMet)

	_, err := s.newService(c).RemoveStorageAttachmentFromAliveUnit(
		c.Context(),
		saUUID,
		false,
		0,
	)

	storageErr, has := errors.AsType[applicationerrors.UnitStorageMinViolation](err)
	c.Check(has, tc.IsTrue)
	c.Check(storageErr, tc.Equals, applicationerrors.UnitStorageMinViolation{
		CharmStorageName: "kratos-key-store",
		RequiredMinimum:  2,
		UnitUUID:         unitUUID.String(),
	})
}

func (s *storageSuite) TestRemoveStorageAttachmentFromAliveUnitNoForceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	detatchInfo := internal.StorageAttachmentDetachInfo{
		CharmStorageName: "kratos-key-store",
		CountFulfilment:  2,
		RequiredCountMin: 1,
		Life:             int(life.Alive),
		UnitLife:         int(life.Alive),
		UnitUUID:         unitUUID.String(),
	}
	exp := s.modelState.EXPECT()
	exp.GetDetachInfoForStorageAttachment(
		gomock.Any(), saUUID.String(),
	).Return(detatchInfo, nil).AnyTimes()
	exp.EnsureStorageAttachmentNotAliveWithFulfilment(
		gomock.Any(), saUUID.String(), 1,
	).Return(nil)
	exp.StorageAttachmentScheduleRemoval(
		gomock.Any(), gomock.Any(), saUUID.String(), false, when.UTC(),
	).Return(nil)

	jobUUID, err := s.newService(c).RemoveStorageAttachmentFromAliveUnit(
		c.Context(),
		saUUID,
		false,
		0,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *storageSuite) TestRemoveStorageAttachmentFromAliveUnitWithForceNoWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when)

	detatchInfo := internal.StorageAttachmentDetachInfo{
		CharmStorageName: "kratos-key-store",
		CountFulfilment:  2,
		RequiredCountMin: 1,
		Life:             int(life.Alive),
		UnitLife:         int(life.Alive),
		UnitUUID:         unitUUID.String(),
	}
	exp := s.modelState.EXPECT()
	exp.GetDetachInfoForStorageAttachment(
		gomock.Any(), saUUID.String(),
	).Return(detatchInfo, nil).AnyTimes()
	exp.EnsureStorageAttachmentNotAliveWithFulfilment(
		gomock.Any(), saUUID.String(), 1,
	).Return(nil)
	exp.StorageAttachmentScheduleRemoval(
		gomock.Any(), gomock.Any(), saUUID.String(), true, when.UTC(),
	).Return(nil)

	jobUUID, err := s.newService(c).RemoveStorageAttachmentFromAliveUnit(
		c.Context(),
		saUUID,
		true,
		0,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *storageSuite) TestRemoveStorageAttachmentFromAliveUnitWithForceWaitSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	detatchInfo := internal.StorageAttachmentDetachInfo{
		CharmStorageName: "kratos-key-store",
		CountFulfilment:  2,
		RequiredCountMin: 1,
		Life:             int(life.Alive),
		UnitLife:         int(life.Alive),
		UnitUUID:         unitUUID.String(),
	}
	exp := s.modelState.EXPECT()
	exp.GetDetachInfoForStorageAttachment(
		gomock.Any(), saUUID.String(),
	).Return(detatchInfo, nil).AnyTimes()
	exp.EnsureStorageAttachmentNotAliveWithFulfilment(
		gomock.Any(), saUUID.String(), 1,
	).Return(nil)

	// The first normal removal scheduled immediately.
	exp.StorageAttachmentScheduleRemoval(
		gomock.Any(), gomock.Any(), saUUID.String(), false, when.UTC(),
	).Return(nil)

	// The forced removal scheduled after the wait duration.
	exp.StorageAttachmentScheduleRemoval(
		gomock.Any(), gomock.Any(), saUUID.String(), true, when.UTC().Add(time.Minute),
	).Return(nil)

	jobUUID, err := s.newService(c).RemoveStorageAttachmentFromAliveUnit(
		c.Context(),
		saUUID,
		true,
		time.Minute,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jobUUID.Validate(), tc.ErrorIsNil)
}

func (s *storageSuite) TestMarkStorageAttachmentAsDeadNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)

	s.modelState.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), uuid.String(),
	).Return(-1, storageerrors.StorageAttachmentNotFound)

	svc := s.newService(c)
	err := svc.MarkStorageAttachmentAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIs, storageerrors.StorageAttachmentNotFound)
}

func (s *storageSuite) TestMarkStorageAttachmentAsDeadStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)

	s.modelState.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), uuid.String(),
	).Return(life.Alive, nil)

	svc := s.newService(c)
	err := svc.MarkStorageAttachmentAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *storageSuite) TestMarkStorageAttachmentAsDeadNoCascade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)

	cascaded := internal.CascadedStorageProvisionedAttachmentLives{}

	s.modelState.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), uuid.String(),
	).Return(life.Dying, nil)
	s.modelState.EXPECT().EnsureStorageAttachmentDeadCascade(
		gomock.Any(), uuid.String(),
	).Return(cascaded, nil)

	svc := s.newService(c)
	err := svc.MarkStorageAttachmentAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestMarkStorageAttachmentAsDeadCascade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC()
	s.clock.EXPECT().Now().Return(now).AnyTimes()

	uuid := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)
	fsaUUID := tc.Must(c, storageprovisioning.NewFilesystemAttachmentUUID)
	vaUUID := tc.Must(c, storageprovisioning.NewFilesystemAttachmentUUID)
	vapUUID := tc.Must(c, storageprovisioning.NewVolumeAttachmentPlanUUID)

	cascaded := internal.CascadedStorageProvisionedAttachmentLives{
		FileSystemAttachmentUUIDs: []string{
			fsaUUID.String(),
		},
		VolumeAttachmentUUIDs: []string{
			vaUUID.String(),
		},
		VolumeAttachmentPlanUUIDs: []string{
			vapUUID.String(),
		},
	}

	s.modelState.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), uuid.String(),
	).Return(life.Dying, nil)
	s.modelState.EXPECT().EnsureStorageAttachmentDeadCascade(
		gomock.Any(), uuid.String(),
	).Return(cascaded, nil)

	s.modelState.EXPECT().FilesystemAttachmentScheduleRemoval(gomock.Any(),
		tc.Bind(tc.IsNonZeroUUID), fsaUUID.String(), false, now).Return(nil)
	s.modelState.EXPECT().VolumeAttachmentScheduleRemoval(gomock.Any(),
		tc.Bind(tc.IsNonZeroUUID), vaUUID.String(), false, now).Return(nil)
	s.modelState.EXPECT().VolumeAttachmentPlanScheduleRemoval(gomock.Any(),
		tc.Bind(tc.IsNonZeroUUID), vapUUID.String(), false, now).Return(nil)

	svc := s.newService(c)
	err := svc.MarkStorageAttachmentAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestRemoveStorageInstanceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storage.NewStorageInstanceUUID)

	cascaded := internal.CascadedStorageFilesystemVolumeLives{}
	s.modelState.EXPECT().EnsureStorageInstanceNotAliveCascade(
		gomock.Any(), uuid.String(), false,
	).Return(cascaded, storageerrors.StorageInstanceNotFound)

	svc := s.newService(c)
	err := svc.RemoveStorageInstance(c.Context(), uuid, false, 0, false)
	c.Assert(err, tc.ErrorIs, storageerrors.StorageInstanceNotFound)
}

func (s *storageSuite) TestRemoveStorageInstance(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC()
	s.clock.EXPECT().Now().Return(now)

	uuid := tc.Must(c, storage.NewStorageInstanceUUID)

	cascaded := internal.CascadedStorageFilesystemVolumeLives{}
	s.modelState.EXPECT().EnsureStorageInstanceNotAliveCascade(
		gomock.Any(), uuid.String(), false,
	).Return(cascaded, nil)
	s.modelState.EXPECT().StorageInstanceScheduleRemoval(
		gomock.Any(), tc.Bind(tc.IsNonZeroUUID), uuid.String(), false, now,
	).Return(nil)

	svc := s.newService(c)
	err := svc.RemoveStorageInstance(c.Context(), uuid, false, 0, false)
	c.Assert(err, tc.ErrorIs, nil)
}

func (s *storageSuite) TestRemoveStorageInstanceCascade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now().UTC()
	s.clock.EXPECT().Now().Return(now).AnyTimes()

	uuid := tc.Must(c, storage.NewStorageInstanceUUID)
	fsUUID := tc.Must(c, storageprovisioning.NewFilesystemUUID).String()
	volUUID := tc.Must(c, storageprovisioning.NewVolumeUUID).String()

	cascaded := internal.CascadedStorageFilesystemVolumeLives{
		FileSystemUUID: &fsUUID,
		VolumeUUID:     &volUUID,
	}
	s.modelState.EXPECT().EnsureStorageInstanceNotAliveCascade(
		gomock.Any(), uuid.String(), false,
	).Return(cascaded, nil)
	s.modelState.EXPECT().StorageInstanceScheduleRemoval(
		gomock.Any(), tc.Bind(tc.IsNonZeroUUID), uuid.String(), false, now,
	).Return(nil)
	s.modelState.EXPECT().FilesystemScheduleRemoval(
		gomock.Any(), tc.Bind(tc.IsNonZeroUUID), fsUUID, false, now,
	).Return(nil)
	s.modelState.EXPECT().VolumeScheduleRemoval(
		gomock.Any(), tc.Bind(tc.IsNonZeroUUID), volUUID, false, now,
	).Return(nil)

	svc := s.newService(c)
	err := svc.RemoveStorageInstance(c.Context(), uuid, false, 0, false)
	c.Assert(err, tc.ErrorIs, nil)
}

func (s *storageSuite) TestExecuteJobForStorageInstanceNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageInstanceJob(c)

	exp := s.modelState.EXPECT()
	exp.GetStorageInstanceLife(
		gomock.Any(), j.EntityUUID,
	).Return(-1, storageerrors.StorageInstanceNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForStorageInstanceStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageInstanceJob(c)

	s.modelState.EXPECT().GetStorageInstanceLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *storageSuite) TestExecuteJobForStorageInstanceHasChildren(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageInstanceJob(c)

	exp := s.modelState.EXPECT()
	exp.GetStorageInstanceLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dying, nil)
	exp.CheckStorageInstanceHasNoChildren(
		gomock.Any(), j.EntityUUID,
	).Return(false, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.StorageInstanceHasChildren)
}

func (s *storageSuite) TestExecuteJobForStorageInstanceForceSkipCheck(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageInstanceJob(c)
	j.Force = true

	exp := s.modelState.EXPECT()
	exp.GetStorageInstanceLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dying, nil)
	exp.DeleteStorageInstance(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String())

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForStorageInstanceSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageInstanceJob(c)

	exp := s.modelState.EXPECT()
	exp.GetStorageInstanceLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dying, nil)
	exp.CheckStorageInstanceHasNoChildren(
		gomock.Any(), j.EntityUUID,
	).Return(true, nil)
	exp.DeleteStorageInstance(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String())

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func newStorageInstanceJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.StorageInstanceJob,
		EntityUUID:  tc.Must(c, storage.NewStorageInstanceUUID).String(),
	}
}

func (s *storageSuite) TestMarkFilesystemAttachmentAsDeadNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storageprovisioning.NewFilesystemAttachmentUUID)

	s.modelState.EXPECT().GetFilesystemAttachmentLife(
		gomock.Any(), uuid.String(),
	).Return(-1, storageprovisioningerrors.FilesystemAttachmentNotFound)

	svc := s.newService(c)
	err := svc.MarkFilesystemAttachmentAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIs,
		storageprovisioningerrors.FilesystemAttachmentNotFound)
}

func (s *storageSuite) TestMarkFilesystemAttachmentAsDeadStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storageprovisioning.NewFilesystemAttachmentUUID)

	s.modelState.EXPECT().GetFilesystemAttachmentLife(
		gomock.Any(), uuid.String(),
	).Return(life.Alive, nil)

	svc := s.newService(c)
	err := svc.MarkFilesystemAttachmentAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *storageSuite) TestMarkFilesystemAttachmentAsDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storageprovisioning.NewFilesystemAttachmentUUID)

	s.modelState.EXPECT().GetFilesystemAttachmentLife(
		gomock.Any(), uuid.String(),
	).Return(life.Dying, nil)
	s.modelState.EXPECT().MarkFilesystemAttachmentAsDead(
		gomock.Any(), uuid.String(),
	).Return(nil)

	svc := s.newService(c)
	err := svc.MarkFilesystemAttachmentAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestMarkVolumeAttachmentAsDeadNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storageprovisioning.NewVolumeAttachmentUUID)

	s.modelState.EXPECT().GetVolumeAttachmentLife(
		gomock.Any(), uuid.String(),
	).Return(-1, storageprovisioningerrors.VolumeAttachmentNotFound)

	svc := s.newService(c)
	err := svc.MarkVolumeAttachmentAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIs,
		storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *storageSuite) TestMarkVolumeAttachmentAsDeadStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storageprovisioning.NewVolumeAttachmentUUID)

	s.modelState.EXPECT().GetVolumeAttachmentLife(
		gomock.Any(), uuid.String(),
	).Return(life.Alive, nil)

	svc := s.newService(c)
	err := svc.MarkVolumeAttachmentAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *storageSuite) TestMarkVolumeAttachmentAsDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storageprovisioning.NewVolumeAttachmentUUID)

	s.modelState.EXPECT().GetVolumeAttachmentLife(
		gomock.Any(), uuid.String(),
	).Return(life.Dying, nil)
	s.modelState.EXPECT().MarkVolumeAttachmentAsDead(
		gomock.Any(), uuid.String(),
	).Return(nil)

	svc := s.newService(c)
	err := svc.MarkVolumeAttachmentAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForFilesystemAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newFilesystemAttachmentJob(c)

	exp := s.modelState.EXPECT()
	exp.GetFilesystemAttachmentLife(
		gomock.Any(), j.EntityUUID,
	).Return(-1, storageprovisioningerrors.FilesystemAttachmentNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForFilesystemAttachmentStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newFilesystemAttachmentJob(c)

	s.modelState.EXPECT().GetFilesystemAttachmentLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *storageSuite) TestExecuteJobForFilesystemAttachmentDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newFilesystemAttachmentJob(c)

	s.modelState.EXPECT().GetFilesystemAttachmentLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dying, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityNotDead)
}

func (s *storageSuite) TestExecuteJobForFilesystemAttachmentDyingForce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newFilesystemAttachmentJob(c)
	j.Force = true

	s.modelState.EXPECT().GetFilesystemAttachmentLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dying, nil)
	s.modelState.EXPECT().DeleteFilesystemAttachment(
		gomock.Any(), j.EntityUUID,
	).Return(nil)
	s.modelState.EXPECT().DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForFilesystemAttachmentSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newFilesystemAttachmentJob(c)

	exp := s.modelState.EXPECT()
	exp.GetFilesystemAttachmentLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dead, nil)
	exp.DeleteFilesystemAttachment(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String())

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func newFilesystemAttachmentJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	fsUUID := tc.Must(c, storageprovisioning.NewFilesystemAttachmentUUID)
	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.StorageFilesystemAttachmentJob,
		EntityUUID:  fsUUID.String(),
	}
}

func (s *storageSuite) TestExecuteJobForVolumeAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newVolumeAttachmentJob(c)

	exp := s.modelState.EXPECT()
	exp.GetVolumeAttachmentLife(
		gomock.Any(), j.EntityUUID,
	).Return(-1, storageprovisioningerrors.VolumeAttachmentNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForVolumeAttachmentStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newVolumeAttachmentJob(c)

	s.modelState.EXPECT().GetVolumeAttachmentLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *storageSuite) TestExecuteJobForVolumeAttachmentDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newVolumeAttachmentJob(c)

	s.modelState.EXPECT().GetVolumeAttachmentLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dying, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityNotDead)
}

func (s *storageSuite) TestExecuteJobForVolumeAttachmentDyingForce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newVolumeAttachmentJob(c)
	j.Force = true

	s.modelState.EXPECT().GetVolumeAttachmentLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dying, nil)
	s.modelState.EXPECT().DeleteVolumeAttachment(
		gomock.Any(), j.EntityUUID,
	).Return(nil)
	s.modelState.EXPECT().DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForVolumeAttachmentSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newVolumeAttachmentJob(c)

	exp := s.modelState.EXPECT()
	exp.GetVolumeAttachmentLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.DeleteVolumeAttachment(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String())

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func newVolumeAttachmentJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.StorageVolumeAttachmentJob,
		EntityUUID:  tc.Must(c, storageprovisioning.NewVolumeAttachmentUUID).String(),
	}
}

func (s *storageSuite) TestMarkVolumeAttachmentPlanAsDeadNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storageprovisioning.NewVolumeAttachmentPlanUUID)

	s.modelState.EXPECT().GetVolumeAttachmentPlanLife(
		gomock.Any(), uuid.String(),
	).Return(-1, storageprovisioningerrors.VolumeAttachmentPlanNotFound)

	svc := s.newService(c)
	err := svc.MarkVolumeAttachmentPlanAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIs,
		storageprovisioningerrors.VolumeAttachmentPlanNotFound)
}

func (s *storageSuite) TestMarkVolumeAttachmentPlanAsDeadStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storageprovisioning.NewVolumeAttachmentPlanUUID)

	s.modelState.EXPECT().GetVolumeAttachmentPlanLife(
		gomock.Any(), uuid.String(),
	).Return(life.Alive, nil)

	svc := s.newService(c)
	err := svc.MarkVolumeAttachmentPlanAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *storageSuite) TestMarkVolumeAttachmentPlanAsDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := tc.Must(c, storageprovisioning.NewVolumeAttachmentPlanUUID)

	s.modelState.EXPECT().GetVolumeAttachmentPlanLife(
		gomock.Any(), uuid.String(),
	).Return(life.Dying, nil)
	s.modelState.EXPECT().MarkVolumeAttachmentPlanAsDead(
		gomock.Any(), uuid.String(),
	).Return(nil)

	svc := s.newService(c)
	err := svc.MarkVolumeAttachmentPlanAsDead(
		c.Context(), uuid,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForVolumeAttachmentPlanNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newVolumeAttachmentPlanJob(c)

	exp := s.modelState.EXPECT()
	exp.GetVolumeAttachmentPlanLife(
		gomock.Any(), j.EntityUUID,
	).Return(-1, storageprovisioningerrors.VolumeAttachmentPlanNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForVolumeAttachmentPlanStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newVolumeAttachmentPlanJob(c)

	s.modelState.EXPECT().GetVolumeAttachmentPlanLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *storageSuite) TestExecuteJobForVolumeAttachmentPlanDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newVolumeAttachmentPlanJob(c)

	s.modelState.EXPECT().GetVolumeAttachmentPlanLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dying, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityNotDead)
}

func (s *storageSuite) TestExecuteJobForVolumeAttachmentPlanDyingForce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newVolumeAttachmentPlanJob(c)
	j.Force = true

	s.modelState.EXPECT().GetVolumeAttachmentPlanLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dying, nil)
	s.modelState.EXPECT().DeleteVolumeAttachmentPlan(
		gomock.Any(), j.EntityUUID,
	).Return(nil)
	s.modelState.EXPECT().DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForVolumeAttachmentPlanSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newVolumeAttachmentPlanJob(c)

	exp := s.modelState.EXPECT()
	exp.GetVolumeAttachmentPlanLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dead, nil)
	exp.DeleteVolumeAttachmentPlan(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String())

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func newVolumeAttachmentPlanJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.StorageVolumeAttachmentPlanJob,
		EntityUUID:  tc.Must(c, storageprovisioning.NewVolumeAttachmentPlanUUID).String(),
	}
}

func (s *storageSuite) TestRemoveStorageAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	saUUID := storageprovtesting.GenStorageAttachmentUUID(c)

	s.modelState.EXPECT().StorageAttachmentExists(gomock.Any(), saUUID.String()).Return(false, nil)

	_, err := s.newService(c).RemoveStorageAttachment(c.Context(), saUUID, false, 0)
	c.Assert(err, tc.ErrorIs, storageerrors.StorageAttachmentNotFound)
}

func (s *storageSuite) TestExecuteJobForStorageAttachmentNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageAttachmentJob(c)

	exp := s.modelState.EXPECT()
	exp.GetStorageAttachmentLife(
		gomock.Any(), j.EntityUUID).Return(-1, storageerrors.StorageAttachmentNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForStorageAttachmentStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageAttachmentJob(c)

	s.modelState.EXPECT().GetStorageAttachmentLife(gomock.Any(), j.EntityUUID).Return(life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *storageSuite) TestExecuteJobForStorageAttachmentDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageAttachmentJob(c)

	s.modelState.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dying, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityNotDead)
}

func (s *storageSuite) TestExecuteJobForStorageAttachmentDyingForce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	when := time.Now().UTC()
	s.clock.EXPECT().Now().Return(when).AnyTimes()

	j := newStorageAttachmentJob(c)
	j.Force = true

	fsaUUID := tc.Must(c, storageprovisioning.NewFilesystemAttachmentUUID)
	vaUUID := tc.Must(c, storageprovisioning.NewVolumeAttachmentUUID)
	vapUUID := tc.Must(c, storageprovisioning.NewVolumeAttachmentPlanUUID)

	cascaded := internal.CascadedStorageProvisionedAttachmentLives{
		FileSystemAttachmentUUIDs: []string{fsaUUID.String()},
		VolumeAttachmentUUIDs:     []string{vaUUID.String()},
		VolumeAttachmentPlanUUIDs: []string{vapUUID.String()},
	}
	s.modelState.EXPECT().GetStorageAttachmentLife(
		gomock.Any(), j.EntityUUID,
	).Return(life.Dying, nil)
	s.modelState.EXPECT().EnsureStorageAttachmentDeadCascade(
		gomock.Any(), j.EntityUUID,
	).Return(cascaded, nil)
	s.modelState.EXPECT().FilesystemAttachmentScheduleRemoval(
		gomock.Any(), tc.Bind(tc.IsNonZeroUUID), fsaUUID.String(), false, when,
	).Return(nil)
	s.modelState.EXPECT().VolumeAttachmentScheduleRemoval(
		gomock.Any(), tc.Bind(tc.IsNonZeroUUID), vaUUID.String(), false, when,
	).Return(nil)
	s.modelState.EXPECT().VolumeAttachmentPlanScheduleRemoval(
		gomock.Any(), tc.Bind(tc.IsNonZeroUUID), vapUUID.String(), false, when,
	).Return(nil)
	s.modelState.EXPECT().DeleteStorageAttachment(
		gomock.Any(), j.EntityUUID,
	).Return(nil)
	s.modelState.EXPECT().DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForStorageAttachmentSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageAttachmentJob(c)

	exp := s.modelState.EXPECT()
	exp.GetStorageAttachmentLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.DeleteStorageAttachment(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String())

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func newStorageAttachmentJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.StorageAttachmentJob,
		EntityUUID:  storageprovtesting.GenStorageAttachmentUUID(c).String(),
	}
}

func (s *storageSuite) TestRemoveDeadFilesystemNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsUUID := tc.Must(c, storageprovisioning.NewFilesystemUUID)

	s.modelState.EXPECT().GetFilesystemLife(
		gomock.Any(), fsUUID.String(),
	).Return(0, storageprovisioningerrors.FilesystemNotFound)

	svc := s.newService(c)
	err := svc.RemoveDeadFilesystem(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

func (s *storageSuite) TestRemoveDeadFilesystemAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsUUID := tc.Must(c, storageprovisioning.NewFilesystemUUID)

	s.modelState.EXPECT().GetFilesystemLife(
		gomock.Any(), fsUUID.String(),
	).Return(life.Alive, nil)

	svc := s.newService(c)
	err := svc.RemoveDeadFilesystem(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotDead)
}

func (s *storageSuite) TestRemoveDeadFilesystemDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsUUID := tc.Must(c, storageprovisioning.NewFilesystemUUID)

	s.modelState.EXPECT().GetFilesystemLife(
		gomock.Any(), fsUUID.String(),
	).Return(life.Dying, nil)

	svc := s.newService(c)
	err := svc.RemoveDeadFilesystem(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotDead)
}

func (s *storageSuite) TestRemoveDeadFilesystem(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsUUID := tc.Must(c, storageprovisioning.NewFilesystemUUID)

	s.modelState.EXPECT().GetFilesystemLife(
		gomock.Any(), fsUUID.String(),
	).Return(life.Dead, nil)
	s.modelState.EXPECT().SetFilesystemStatus(
		gomock.Any(), fsUUID.String(),
		int(domainstatus.StorageFilesystemStatusTypeTombstone),
	).Return(nil)

	svc := s.newService(c)
	err := svc.RemoveDeadFilesystem(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForStorageFilesystemNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageFilesystemJob(c)

	exp := s.modelState.EXPECT()
	exp.GetFilesystemLife(gomock.Any(), j.EntityUUID).Return(
		-1, storageprovisioningerrors.FilesystemNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForStorageFilesystemStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageFilesystemJob(c)

	s.modelState.EXPECT().GetFilesystemLife(gomock.Any(), j.EntityUUID).Return(
		life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *storageSuite) TestExecuteJobForStorageFilesystemDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageFilesystemJob(c)

	exp := s.modelState.EXPECT()
	exp.GetFilesystemLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityNotDead)
}

func (s *storageSuite) TestExecuteJobForStorageFilesystemNotTombstone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageFilesystemJob(c)

	exp := s.modelState.EXPECT()
	exp.GetFilesystemLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetFilesystemStatus(
		gomock.Any(), j.EntityUUID,
	).Return(int(domainstatus.StorageFilesystemStatusTypeDestroying), nil)
	exp.CheckVolumeBackedFilesystemCrossProvisioned(
		gomock.Any(), j.EntityUUID,
	).Return(false, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.StorageFilesystemNoTombstone)
}

func (s *storageSuite) TestExecuteJobForStorageFilesystemNotTombstoneVolumeBackedCrossProvisioned(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageFilesystemJob(c)

	exp := s.modelState.EXPECT()
	exp.GetFilesystemLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetFilesystemStatus(
		gomock.Any(), j.EntityUUID,
	).Return(int(domainstatus.StorageFilesystemStatusTypeDestroying), nil)
	exp.CheckVolumeBackedFilesystemCrossProvisioned(
		gomock.Any(), j.EntityUUID,
	).Return(true, nil)
	exp.DeleteFilesystem(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String())

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForStorageFilesystemSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageFilesystemJob(c)

	exp := s.modelState.EXPECT()
	exp.GetFilesystemLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetFilesystemStatus(
		gomock.Any(), j.EntityUUID,
	).Return(int(domainstatus.StorageFilesystemStatusTypeTombstone), nil)
	exp.DeleteFilesystem(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String())

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func newStorageFilesystemJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.StorageFilesystemJob,
		EntityUUID:  tc.Must(c, storageprovisioning.NewFilesystemUUID).String(),
	}
}

func (s *storageSuite) TestRemoveDeadVolumeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fsUUID := tc.Must(c, storageprovisioning.NewVolumeUUID)

	s.modelState.EXPECT().GetVolumeLife(
		gomock.Any(), fsUUID.String(),
	).Return(0, storageprovisioningerrors.VolumeNotFound)

	svc := s.newService(c)
	err := svc.RemoveDeadVolume(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *storageSuite) TestRemoveDeadVolumeAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	volUUID := tc.Must(c, storageprovisioning.NewVolumeUUID)

	s.modelState.EXPECT().GetVolumeLife(
		gomock.Any(), volUUID.String(),
	).Return(life.Alive, nil)

	svc := s.newService(c)
	err := svc.RemoveDeadVolume(c.Context(), volUUID)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotDead)
}

func (s *storageSuite) TestRemoveDeadVolumeDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	volUUID := tc.Must(c, storageprovisioning.NewVolumeUUID)

	s.modelState.EXPECT().GetVolumeLife(
		gomock.Any(), volUUID.String(),
	).Return(life.Dying, nil)

	svc := s.newService(c)
	err := svc.RemoveDeadVolume(c.Context(), volUUID)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotDead)
}

func (s *storageSuite) TestRemoveDeadVolume(c *tc.C) {
	defer s.setupMocks(c).Finish()

	volUUID := tc.Must(c, storageprovisioning.NewVolumeUUID)

	s.modelState.EXPECT().GetVolumeLife(
		gomock.Any(), volUUID.String(),
	).Return(life.Dead, nil)
	s.modelState.EXPECT().SetVolumeStatus(
		gomock.Any(), volUUID.String(),
		int(domainstatus.StorageVolumeStatusTypeTombstone),
	).Return(nil)

	svc := s.newService(c)
	err := svc.RemoveDeadVolume(c.Context(), volUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForStorageVolumeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageVolumeJob(c)

	exp := s.modelState.EXPECT()
	exp.GetVolumeLife(gomock.Any(), j.EntityUUID).Return(
		-1, storageprovisioningerrors.VolumeNotFound)
	exp.DeleteJob(gomock.Any(), j.UUID.String()).Return(nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestExecuteJobForStorageVolumeStillAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageVolumeJob(c)

	s.modelState.EXPECT().GetVolumeLife(gomock.Any(), j.EntityUUID).Return(
		life.Alive, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityStillAlive)
}

func (s *storageSuite) TestExecuteJobForStorageVolumeDying(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageVolumeJob(c)

	s.modelState.EXPECT().GetVolumeLife(gomock.Any(), j.EntityUUID).Return(
		life.Dying, nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.EntityNotDead)
}

func (s *storageSuite) TestExecuteJobForStorageVolumeNotTombstone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageVolumeJob(c)

	exp := s.modelState.EXPECT()
	exp.GetVolumeLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetVolumeStatus(
		gomock.Any(), j.EntityUUID,
	).Return(int(domainstatus.StorageVolumeStatusTypeDestroying), nil)

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIs, removalerrors.StorageVolumeNoTombstone)
}

func (s *storageSuite) TestExecuteJobForStorageVolumeSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageVolumeJob(c)

	exp := s.modelState.EXPECT()
	exp.GetVolumeLife(gomock.Any(), j.EntityUUID).Return(life.Dead, nil)
	exp.GetVolumeStatus(
		gomock.Any(), j.EntityUUID,
	).Return(int(domainstatus.StorageVolumeStatusTypeTombstone), nil)
	exp.DeleteVolume(gomock.Any(), j.EntityUUID).Return(nil)
	exp.DeleteJob(gomock.Any(), j.UUID.String())

	err := s.newService(c).ExecuteJob(c.Context(), j)
	c.Assert(err, tc.ErrorIsNil)
}

func newStorageVolumeJob(c *tc.C) removal.Job {
	jUUID, err := removal.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	return removal.Job{
		UUID:        jUUID,
		RemovalType: removal.StorageVolumeJob,
		EntityUUID:  tc.Must(c, storageprovisioning.NewVolumeUUID).String(),
	}
}
