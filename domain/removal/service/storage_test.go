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
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storageprovisioning"
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

func (s *storageSuite) TestExecuteJobForStorageAttachmentSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	j := newStorageAttachmentJob(c)

	exp := s.modelState.EXPECT()
	exp.GetStorageAttachmentLife(gomock.Any(), j.EntityUUID).Return(life.Dying, nil)
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

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	s.modelState.EXPECT().GetFilesystemLife(
		gomock.Any(), fsUUID.String(),
	).Return(life.Dead, nil)
	s.modelState.EXPECT().FilesystemScheduleRemoval(
		gomock.Any(), gomock.Any(), fsUUID.String(), false, when.UTC(),
	).Return(nil)

	svc := s.newService(c)
	err := svc.RemoveDeadFilesystem(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIsNil)
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

	when := time.Now()
	s.clock.EXPECT().Now().Return(when).MinTimes(1)

	s.modelState.EXPECT().GetVolumeLife(
		gomock.Any(), volUUID.String(),
	).Return(life.Dead, nil)
	s.modelState.EXPECT().VolumeScheduleRemoval(
		gomock.Any(), gomock.Any(), volUUID.String(), false, when.UTC(),
	).Return(nil)

	svc := s.newService(c)
	err := svc.RemoveDeadVolume(c.Context(), volUUID)
	c.Assert(err, tc.ErrorIsNil)
}
