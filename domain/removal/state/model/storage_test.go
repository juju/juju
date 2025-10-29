// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	corenetwork "github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/network"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type storageSuite struct {
	schematesting.ModelSuite
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestStorageAttachmentExists(c *tc.C) {
	ctx := c.Context()

	_, attachment := s.addAppUnitStorage(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	exists, err := st.StorageAttachmentExists(ctx, attachment)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)

	exists, err = st.StorageAttachmentExists(ctx, "not-today-henry")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *storageSuite) TestEnsureStorageAttachmentNotAliveSuccess(c *tc.C) {
	_, attachment := s.addAppUnitStorage(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	ctx := c.Context()

	err := st.EnsureStorageAttachmentNotAlive(ctx, attachment)
	c.Assert(err, tc.ErrorIsNil)

	// Attachment had life "alive" and should now be "dying".
	row := s.DB().QueryRowContext(ctx, "SELECT life_id FROM storage_attachment WHERE uuid = ?", attachment)
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)

	// Idempotent. A "dying" attachment is a no-op. Life is unchanged.
	err = st.EnsureStorageAttachmentNotAlive(ctx, attachment)
	c.Assert(err, tc.ErrorIsNil)

	row = s.DB().QueryRow("SELECT life_id FROM storage_attachment WHERE uuid = ?", attachment)
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

func (s *storageSuite) TestScheduleStorageAttachmentRemovalSuccess(c *tc.C) {
	_, attachment := s.addAppUnitStorage(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.StorageAttachmentScheduleRemoval(
		c.Context(), "removal-uuid", attachment, false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	row := s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal WHERE uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalTypeID, tc.Equals, 6)
	c.Check(rUUID, tc.Equals, attachment)
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *storageSuite) TestGetStorageAttachentLifeSuccess(c *tc.C) {
	_, saUUID := s.addAppUnitStorage(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetStorageAttachmentLife(c.Context(), saUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Alive)
}

func (s *storageSuite) TestGetStorageAttachmentLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetStorageAttachmentLife(c.Context(), "some-sa-uuid")
	c.Assert(err, tc.ErrorIs, storageerrors.StorageAttachmentNotFound)
}

func (s *storageSuite) TestEnsureStorageAttachmentDeadCascadeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	saUUID := "some-storage-attachment-uuid"
	_, err := st.EnsureStorageAttachmentDeadCascade(c.Context(), saUUID)
	c.Assert(err, tc.ErrorIs, storageerrors.StorageAttachmentNotFound)
}

func (s *storageSuite) TestEnsureStorageAttachmentDeadCascade(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	siUUID, saUUID := s.addAppUnitStorage(c)
	volUUID, vaUUID, vapUUID := s.addAttachedVolumeWithPlan(c)
	s.addStorageInstanceVolume(c, siUUID, volUUID)
	fsUUID, fsaUUID := s.addAttachedFilesystem(c)
	s.addStorageInstanceFilesystem(c, siUUID, fsUUID)
	s.setStorageAttachmentLife(c, saUUID, 1)

	cascaded, err := st.EnsureStorageAttachmentDeadCascade(c.Context(), saUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cascaded, tc.DeepEquals,
		internal.CascadedStorageProvisionedAttachmentLives{
			FileSystemAttachmentUUIDs: []string{fsaUUID},
			VolumeAttachmentUUIDs:     []string{vaUUID},
			VolumeAttachmentPlanUUIDs: []string{vapUUID},
		},
	)

	res := s.DB().QueryRow(
		"SELECT life_id FROM storage_attachment WHERE uuid = ?", saUUID)
	var lifeId int
	c.Assert(res.Scan(&lifeId), tc.ErrorIsNil)
	c.Check(lifeId, tc.Equals, 2)
}

func (s *storageSuite) TestDeleteStorageAttachmentSuccess(c *tc.C) {
	siUUID, saUUID := s.addAppUnitStorage(c)

	ctx := c.Context()

	err := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c)).DeleteStorageAttachment(ctx, saUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Attachment is gone.
	var dummy string
	row := s.DB().QueryRowContext(ctx, "SELECT uuid FROM storage_attachment WHERE uuid = ?", saUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)

	// The attached unit was the owner, so the owner record is gone.
	row = s.DB().QueryRowContext(
		ctx, "SELECT unit_uuid FROM storage_unit_owner WHERE storage_instance_uuid = ?", siUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
}

func (s *storageSuite) TestGetVolumeLife(c *tc.C) {
	volUUID := s.addVolume(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetVolumeLife(c.Context(), volUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Alive)
}

func (s *storageSuite) TestGetVolumeLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetVolumeLife(c.Context(), "some-vol-uuid")
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *storageSuite) TestVolumeScheduleRemovalSuccess(c *tc.C) {
	volUUID := s.addVolume(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.VolumeScheduleRemoval(
		c.Context(), "removal-uuid", volUUID, false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	row := s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal WHERE uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalTypeID, tc.Equals, 7)
	c.Check(rUUID, tc.Equals, volUUID)
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *storageSuite) TestDeleteVolume(c *tc.C) {
	volUUID := s.addVolume(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteVolume(c.Context(), volUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Volume is gone.
	var dummy string
	row := s.DB().QueryRow(
		"SELECT uuid FROM storage_volume WHERE uuid = ?", volUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
}

func (s *storageSuite) TestDeleteVolumeWithInstance(c *tc.C) {
	siUUID, _ := s.addAppUnitStorage(c)
	volUUID := s.addVolume(c)
	s.bindVolumeToStorageInstance(c, siUUID, volUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteVolume(c.Context(), volUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Volume is gone.
	var dummy string
	row := s.DB().QueryRow(
		"SELECT uuid FROM storage_volume WHERE uuid = ?", volUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
	// Storage instance volume is gone.
	row = s.DB().QueryRow(
		"SELECT storage_volume_uuid FROM storage_instance_volume WHERE storage_volume_uuid = ?",
		volUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
}

func (s *storageSuite) TestGetFilesystemLife(c *tc.C) {
	fsUUID := s.addFilesystem(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	l, err := st.GetFilesystemLife(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(l, tc.Equals, life.Alive)
}

func (s *storageSuite) TestGetFilesystemLifeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	_, err := st.GetFilesystemLife(c.Context(), "some-vol-uuid")
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.FilesystemNotFound)
}

func (s *storageSuite) TestFilesystemScheduleRemovalSuccess(c *tc.C) {
	fsUUID := s.addFilesystem(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	when := time.Now().UTC()
	err := st.FilesystemScheduleRemoval(
		c.Context(), "removal-uuid", fsUUID, false, when,
	)
	c.Assert(err, tc.ErrorIsNil)

	// We should have a removal job scheduled immediately.
	row := s.DB().QueryRow(
		"SELECT removal_type_id, entity_uuid, force, scheduled_for FROM removal WHERE uuid = ?",
		"removal-uuid",
	)
	var (
		removalTypeID int
		rUUID         string
		force         bool
		scheduledFor  time.Time
	)
	err = row.Scan(&removalTypeID, &rUUID, &force, &scheduledFor)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(removalTypeID, tc.Equals, 8)
	c.Check(rUUID, tc.Equals, fsUUID)
	c.Check(force, tc.Equals, false)
	c.Check(scheduledFor, tc.Equals, when)
}

func (s *storageSuite) TestDeleteFilesystem(c *tc.C) {
	fsUUID := s.addFilesystem(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteFilesystem(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Filesystem is gone.
	var dummy string
	row := s.DB().QueryRow(
		"SELECT uuid FROM storage_filesystem WHERE uuid = ?", fsUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
}

func (s *storageSuite) TestDeleteFilesystemWithInstance(c *tc.C) {
	siUUID, _ := s.addAppUnitStorage(c)
	fsUUID := s.addFilesystem(c)
	s.bindFilesystemToStorageInstance(c, siUUID, fsUUID)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))

	err := st.DeleteFilesystem(c.Context(), fsUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Filesystem is gone.
	var dummy string
	row := s.DB().QueryRow(
		"SELECT uuid FROM storage_filesystem WHERE uuid = ?", fsUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
	// Storage instance filesystem is gone.
	row = s.DB().QueryRow(
		"SELECT storage_filesystem_uuid FROM storage_instance_filesystem WHERE storage_filesystem_uuid = ?",
		fsUUID)
	c.Check(row.Scan(&dummy), tc.ErrorIs, sql.ErrNoRows)
}

func (s *storageSuite) TestMarkFilesystemAttachmentAsDeadNotFound(c *tc.C) {
	ctx := c.Context()

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.MarkFilesystemAttachmentAsDead(ctx, "some-fsa-uuid")
	c.Assert(err, tc.ErrorIs,
		storageprovisioningerrors.FilesystemAttachmentNotFound)
}

func (s *storageSuite) TestMarkFilesystemAttachmentAsDead(c *tc.C) {
	ctx := c.Context()

	_, fsaUUID := s.addAttachedFilesystem(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.MarkFilesystemAttachmentAsDead(ctx, fsaUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Filesystem Attachment should be Dead
	row := s.DB().QueryRowContext(ctx,
		"SELECT life_id FROM storage_filesystem_attachment WHERE uuid = ?",
		fsaUUID,
	)
	var lifeID int
	c.Check(row.Scan(&lifeID), tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 2)
}

func (s *storageSuite) TestMarkVolumeAttachmentAsDeadNotFound(c *tc.C) {
	ctx := c.Context()

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.MarkVolumeAttachmentAsDead(ctx, "some-va-uuid")
	c.Assert(err, tc.ErrorIs,
		storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *storageSuite) TestMarkVolumeAttachmentAsDead(c *tc.C) {
	ctx := c.Context()

	_, vaUUID := s.addAttachedVolume(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.MarkVolumeAttachmentAsDead(ctx, vaUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Volume Attachment should be Dead
	row := s.DB().QueryRowContext(ctx,
		"SELECT life_id FROM storage_volume_attachment WHERE uuid = ?", vaUUID,
	)
	var lifeID int
	c.Check(row.Scan(&lifeID), tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 2)
}

func (s *storageSuite) TestMarkVolumeAttachmentPlanAsDeadNotFound(c *tc.C) {
	ctx := c.Context()

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.MarkVolumeAttachmentPlanAsDead(ctx, "some-vap-uuid")
	c.Assert(err, tc.ErrorIs,
		storageprovisioningerrors.VolumeAttachmentPlanNotFound)
}

func (s *storageSuite) TestMarkVolumeAttachmentPlanAsDead(c *tc.C) {
	ctx := c.Context()

	_, _, vapUUID := s.addAttachedVolumeWithPlan(c)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.MarkVolumeAttachmentPlanAsDead(ctx, vapUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Volume Attachment Plan should be Dead
	row := s.DB().QueryRowContext(ctx,
		"SELECT life_id FROM storage_volume_attachment_plan WHERE uuid = ?",
		vapUUID,
	)
	var lifeID int
	c.Check(row.Scan(&lifeID), tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 2)
}

func (s *storageSuite) TestGetDetachInfoForStorageAttachmentNotFound(c *tc.C) {
	saUUID := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	_, err := st.GetDetachInfoForStorageAttachment(c.Context(), saUUID.String())
	c.Check(err, tc.ErrorIs, storageerrors.StorageAttachmentNotFound)
}

// TestGetDetachInfoForStorageAttachmentSuccess tests that when asking for the
// detach information of a storage attachment state correctly reports backs what
// we expect and properly counts the fulfilment against the charm storage.
func (s *storageSuite) TestGetDetachInfoForStorageAttachmentSuccess(c *tc.C) {
	unitUUID, attachments := s.addAppUnitWithCharmStorage(
		c, map[string]charmStorage{
			"data": {CharmMin: 1, Fulfilment: 3},
		},
	)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	info, err := st.GetDetachInfoForStorageAttachment(
		c.Context(), attachments["data"][2],
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(info, tc.Equals, internal.StorageAttachmentDetachInfo{
		CharmStorageName: "data",
		CountFulfilment:  3,
		RequiredCountMin: 1,
		Life:             0,
		UnitUUID:         unitUUID,
		UnitLife:         0,
	})
}

// TestGetDetachInfoForStorageAttachmentMultiple is about testing the correct
// detach information when the unit has many storage attachments for different
// chamr storage definitions.
func (s *storageSuite) TestGetDetachInfoForStorageAttachmentMultiple(c *tc.C) {
	unitUUID, attachments := s.addAppUnitWithCharmStorage(
		c, map[string]charmStorage{
			"data1": {CharmMin: 1, Fulfilment: 1},
			"data2": {CharmMin: 2, Fulfilment: 2},
		},
	)

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	info, err := st.GetDetachInfoForStorageAttachment(
		c.Context(), attachments["data2"][0],
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(info, tc.Equals, internal.StorageAttachmentDetachInfo{
		CharmStorageName: "data2",
		CountFulfilment:  2,
		RequiredCountMin: 2,
		Life:             0,
		UnitUUID:         unitUUID,
		UnitLife:         0,
	})
}

// TestGetDetachInfoForStorageAttachmentExcludeDeadAttachment tests that when
// the unit has one or more dead storage attachments they are not included in
// the count under fulfilment.
func (s *storageSuite) TestGetDetachInfoForStorageAttachmentExcludeDeadAttachment(c *tc.C) {
	unitUUID, attachments := s.addAppUnitWithCharmStorage(
		c, map[string]charmStorage{
			"data": {CharmMin: 1, Fulfilment: 3},
		},
	)
	s.setStorageAttachmentDead(c, attachments["data"][1])

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	info, err := st.GetDetachInfoForStorageAttachment(
		c.Context(), attachments["data"][2],
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(info, tc.Equals, internal.StorageAttachmentDetachInfo{
		CharmStorageName: "data",
		CountFulfilment:  2,
		RequiredCountMin: 1,
		Life:             0,
		UnitUUID:         unitUUID,
		UnitLife:         0,
	})
}

// TestEnsureStorageAttachmentNotAliveWithFulfilment tests the happy path of
// removing a single storage attachment off a unit with a correct fulfilment.
func (s *storageSuite) TestEnsureStorageAttachmentNotAliveWithFulfilment(c *tc.C) {
	_, attachments := s.addAppUnitWithCharmStorage(
		c, map[string]charmStorage{
			"data": {CharmMin: 1, Fulfilment: 3},
		},
	)
	ctx := c.Context()
	attachmentUUID := attachments["data"][1]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.EnsureStorageAttachmentNotAliveWithFulfilment(
		ctx, attachmentUUID, 2,
	)

	c.Check(err, tc.ErrorIsNil)
	row := s.DB().QueryRowContext(ctx, "SELECT life_id FROM storage_attachment WHERE uuid = ?", attachmentUUID)
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

// TestEnsureStorageAttachmentNotAliveWithFulfilmentNotMet tests the scenario
// where the expected fulfilment of the caller is not met. This would be due to
// a unit's storage changing. The caller should expect a error back that
// satisfies [removalerrors.StorageFulfilmentNotMet] and the attachment MUST
// have no life change.
func (s *storageSuite) TestEnsureStorageAttachmentNotAliveWithFulfilmentNotMet(c *tc.C) {
	_, attachments := s.addAppUnitWithCharmStorage(
		c, map[string]charmStorage{
			"data": {CharmMin: 1, Fulfilment: 3},
		},
	)
	ctx := c.Context()
	attachmentUUID := attachments["data"][1]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.EnsureStorageAttachmentNotAliveWithFulfilment(
		ctx, attachmentUUID, 1,
	)
	c.Check(err, tc.ErrorIs, removalerrors.StorageFulfilmentNotMet)

	// Check that the attachment is still alive.
	row := s.DB().QueryRowContext(ctx, "SELECT life_id FROM storage_attachment WHERE uuid = ?", attachmentUUID)
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 0)
}

// TestEnsureStorageAttachmentNotAliveWithFulfilmentOnlyAlive tests that when
// calculating fulfilment for a storage attachment removal only attachments
// which are alive are considered.
func (s *storageSuite) TestEnsureStorageAttachmentNotAliveWithFulfilmentOnlyAlive(c *tc.C) {
	_, attachments := s.addAppUnitWithCharmStorage(
		c, map[string]charmStorage{
			"data": {CharmMin: 1, Fulfilment: 3},
		},
	)
	s.setStorageAttachmentDead(c, attachments["data"][0])
	ctx := c.Context()
	attachmentUUID := attachments["data"][1]

	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.EnsureStorageAttachmentNotAliveWithFulfilment(
		ctx, attachmentUUID, 1,
	)
	c.Check(err, tc.ErrorIsNil)

	// Check that the attachment is still alive.
	row := s.DB().QueryRowContext(ctx, "SELECT life_id FROM storage_attachment WHERE uuid = ?", attachmentUUID)
	var lifeID int
	err = row.Scan(&lifeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(lifeID, tc.Equals, 1)
}

// TestEnsureStorageAttachmentNotAliveWithFulfilmentNoop tests that when the
// storage attachment does exist the operation just becomes a noop and the
// fulfilment is ignored.
func (s *storageSuite) TestEnsureStorageAttachmentNotAliveWithFulfilmentNoop(c *tc.C) {
	notFoundUUID := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)
	st := NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
	err := st.EnsureStorageAttachmentNotAliveWithFulfilment(
		c.Context(), notFoundUUID.String(), 100,
	)
	c.Check(err, tc.ErrorIsNil)
}

type charmStorage struct {
	CharmMin   int
	Fulfilment int
}

// addAppUnitWithCharmStorage sets up a unit in the model with associated charm
// storage that matches the map key and the minimum count set to the supplied
// int.
func (s *storageSuite) addAppUnitWithCharmStorage(
	c *tc.C, charmStorage map[string]charmStorage,
) (string, map[string][]string) {
	appUUID := tc.Must(c, coreapplication.NewUUID)
	charmUUID := tc.Must(c, corecharm.NewID)
	storagePoolUUID := tc.Must(c, storage.NewStoragePoolUUID)
	unitUUID := tc.Must(c, coreunit.NewUUID)
	unitNetNodeUUID := tc.Must(c, network.NewNetNodeUUID)

	ctx := c.Context()

	_, err := s.DB().ExecContext(
		ctx,
		"INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, 0)",
		charmUUID.String(), "testcharm",
	)
	c.Assert(err, tc.ErrorIsNil)

	// create charm storage records
	for name, cs := range charmStorage {
		_, err = s.DB().ExecContext(
			ctx,
			`
INSERT INTO charm_storage (charm_uuid, name, storage_kind_id, count_min, count_max)
VALUES (?, ?, 1, ?, -1)
`,
			charmUUID.String(), name, cs.CharmMin,
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	_, err = s.DB().ExecContext(
		ctx,
		"INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid) VALUES (?, ?, ?, 0, ?)",
		appUUID.String(), charmUUID, "testapp", corenetwork.AlphaSpaceId,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		ctx, "INSERT INTO net_node VALUES (?)", unitNetNodeUUID.String())
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		ctx,
		`
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)
`,
		unitUUID.String(),
		"testapp/0",
		appUUID.String(),
		charmUUID.String(),
		unitNetNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		ctx,
		`
INSERT INTO storage_pool (uuid, name, type)
VALUES (?, 'removal-storage-pool', 'removal-storage-provider')
`,
		storagePoolUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	rval := make(map[string][]string, len(charmStorage))
	createUnitStorage := func(name string) {
		siUUID := tc.Must(c, storage.NewStorageInstanceUUID)
		_, err = s.DB().ExecContext(
			ctx,
			`
INSERT INTO storage_instance (uuid, charm_name, storage_name, storage_kind_id,
                              storage_id, life_id, storage_pool_uuid,
                              requested_size_mib)
VALUES (?, 'testcharm', ?, 1, ?, 1, ?, 1024)
`,
			siUUID.String(),
			name,
			siUUID.String(),
			storagePoolUUID.String(),
		)
		c.Assert(err, tc.ErrorIsNil)

		saUUID := tc.Must(c, storageprovisioning.NewStorageAttachmentUUID)
		_, err = s.DB().ExecContext(
			ctx,
			`
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, ?, 0)
			`,
			saUUID.String(), siUUID.String(), unitUUID.String(),
		)
		c.Assert(err, tc.ErrorIsNil)

		rval[name] = append(rval[name], saUUID.String())
	}

	for name, cs := range charmStorage {
		for range cs.Fulfilment {
			createUnitStorage(name)
		}
	}

	return unitUUID.String(), rval
}

// addAppUnitStorage sets up a unit with a storage attachment.
// The storage instance and attachment UUIDs are returned.
func (s *storageSuite) addAppUnitStorage(c *tc.C) (string, string) {
	ctx := c.Context()

	charm := "some-charm-uuid"
	_, err := s.DB().ExecContext(ctx, "INSERT INTO charm (uuid, reference_name, architecture_id) VALUES (?, ?, ?)",
		charm, charm, 0)
	c.Assert(err, tc.ErrorIsNil)

	app := "some-app-uuid"
	_, err = s.DB().ExecContext(
		ctx, "INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)",
		app, app, 0, charm, corenetwork.AlphaSpaceId,
	)
	c.Assert(err, tc.ErrorIsNil)

	node := "some-net-node-uuid"
	_, err = s.DB().ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)", node)
	c.Assert(err, tc.ErrorIsNil)

	unit := "some-unit-uuid"
	_, err = s.DB().ExecContext(
		ctx,
		"INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)",
		unit, unit, 0, app, charm, node)
	c.Assert(err, tc.ErrorIsNil)

	storagePool := "some-storage-pool-uuid"
	_, err = s.DB().ExecContext(ctx, "INSERT INTO storage_pool (uuid, name, type) VALUES (?, ?, ?)",
		storagePool, "loop", "loop")
	c.Assert(err, tc.ErrorIsNil)

	storageInstance := "some-storage-instance-uuid"
	_, err = s.DB().Exec(`
INSERT INTO storage_instance (
    uuid, storage_id, storage_kind_id, storage_pool_uuid, requested_size_mib,
    charm_name, storage_name, life_id
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		storageInstance, charm+"/0", 1, storagePool, 100, charm, "storage", 0)
	c.Assert(err, tc.ErrorIsNil)

	storageAttachment := "some-storage-attachment-uuid"
	_, err = s.DB().ExecContext(ctx,
		"INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id) VALUES (?, ?, ?, ?)",
		storageAttachment, storageInstance, unit, 0)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(ctx,
		"INSERT INTO storage_unit_owner (storage_instance_uuid, unit_uuid) VALUES (?, ?)", storageInstance, unit)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstance, storageAttachment
}

func (s *storageSuite) addVolume(c *tc.C) string {
	ctx := c.Context()

	volUUID := "some-vol-uuid"
	_, err := s.DB().ExecContext(ctx,
		"INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id) VALUES (?, ?, ?, ?)",
		volUUID, "some-vol", 0, 0,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(ctx,
		"INSERT INTO storage_volume_status (volume_uuid, status_id) VALUES (?, ?)",
		volUUID, 0,
	)
	c.Assert(err, tc.ErrorIsNil)

	return volUUID
}

func (s *storageSuite) bindVolumeToStorageInstance(c *tc.C, siUUID, volUUID string) {
	ctx := c.Context()

	_, err := s.DB().ExecContext(ctx,
		"INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid) VALUES (?, ?)",
		siUUID, volUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) addFilesystem(c *tc.C) string {
	ctx := c.Context()

	fsUUID := "some-fs-uuid"
	_, err := s.DB().ExecContext(ctx,
		"INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id) VALUES (?, ?, ?, ?)",
		fsUUID, "some-fs", 0, 0,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(ctx,
		"INSERT INTO storage_filesystem_status (filesystem_uuid, status_id) VALUES (?, ?)",
		fsUUID, 0,
	)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID
}

func (s *storageSuite) bindFilesystemToStorageInstance(c *tc.C, siUUID, fsUUID string) {
	ctx := c.Context()

	_, err := s.DB().ExecContext(ctx,
		"INSERT INTO storage_instance_filesystem (storage_instance_uuid, storage_filesystem_uuid) VALUES (?, ?)",
		siUUID, fsUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
}

// addAttachedFilesystem adds a filesystem, a net node and attaches the two.
// It returns the filesystem UUID and the filesystem attachment UUID.
func (s *storageSuite) addAttachedFilesystem(c *tc.C) (string, string) {
	ctx := c.Context()

	netNodeUUID := "some-net-node-uuid"
	_, err := s.DB().ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)",
		netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	fsUUID := "some-fs-uuid"
	_, err = s.DB().ExecContext(ctx, "INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id) VALUES (?, ?, ?, ?)",
		fsUUID, "some-fs", 0, 0)
	c.Assert(err, tc.ErrorIsNil)

	fsaUUID := "some-fsa-uuid"
	_, err = s.DB().ExecContext(ctx, "INSERT INTO storage_filesystem_attachment (uuid, storage_filesystem_uuid, net_node_uuid, life_id, provision_scope_id) VALUES (?, ?, ?, ?, ?)",
		fsaUUID, fsUUID, netNodeUUID, 0, 0)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID, fsaUUID
}

// addAttachedVolume adds a volume, a net node and attaches the two.
// It returns the volume UUID and the volume attachment UUID.
func (s *storageSuite) addAttachedVolume(c *tc.C) (string, string) {
	ctx := c.Context()

	netNodeUUID := "some-net-node-uuid"
	_, err := s.DB().ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)",
		netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	volUUID := "some-vol-uuid"
	_, err = s.DB().ExecContext(ctx, "INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id) VALUES (?, ?, ?, ?)",
		volUUID, "some-vol", 0, 0)
	c.Assert(err, tc.ErrorIsNil)

	vaUUID := "some-va-uuid"
	_, err = s.DB().ExecContext(ctx, "INSERT INTO storage_volume_attachment (uuid, storage_volume_uuid, net_node_uuid, life_id, provision_scope_id) VALUES (?, ?, ?, ?, ?)",
		vaUUID, volUUID, netNodeUUID, 0, 0)
	c.Assert(err, tc.ErrorIsNil)

	return volUUID, vaUUID
}

// addAttachedVolumeWithPlan adds a volume, a net node and attaches the two with
// an attachment plan as well.
// It returns the volume UUID, the volume attachment UUID and the volume
// attachment plan UUID.
func (s *storageSuite) addAttachedVolumeWithPlan(c *tc.C) (string, string, string) {
	ctx := c.Context()

	netNodeUUID := "some-net-node-uuid"
	_, err := s.DB().ExecContext(ctx, "INSERT INTO net_node (uuid) VALUES (?)",
		netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	volUUID := "some-vol-uuid"
	_, err = s.DB().ExecContext(ctx, "INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id) VALUES (?, ?, ?, ?)",
		volUUID, "some-vol", 0, 0)
	c.Assert(err, tc.ErrorIsNil)

	vaUUID := "some-va-uuid"
	_, err = s.DB().ExecContext(ctx, "INSERT INTO storage_volume_attachment (uuid, storage_volume_uuid, net_node_uuid, life_id, provision_scope_id) VALUES (?, ?, ?, ?, ?)",
		vaUUID, volUUID, netNodeUUID, 0, 0)
	c.Assert(err, tc.ErrorIsNil)

	vapUUID := "some-vap-uuid"
	_, err = s.DB().ExecContext(ctx, "INSERT INTO storage_volume_attachment_plan (uuid, storage_volume_uuid, net_node_uuid, life_id, provision_scope_id) VALUES (?, ?, ?, ?, ?)",
		vapUUID, volUUID, netNodeUUID, 0, 0)
	c.Assert(err, tc.ErrorIsNil)

	return volUUID, vaUUID, vapUUID
}

// setStorageAttachmentDead is a testing helper method for setting the life of
// a storage attachment to "dead".
func (s *storageSuite) setStorageAttachmentDead(c *tc.C, saUUID string) {
	_, err := s.DB().ExecContext(
		c.Context(),
		"UPDATE storage_attachment SET life_id = 1 WHERE uuid = ?",
		saUUID,
	)
}

func (s *storageSuite) setStorageAttachmentLife(
	c *tc.C, saUUID string, lifeId int,
) {
	_, err := s.DB().Exec("UPDATE storage_attachment SET life_id = ? WHERE uuid = ?", lifeId, saUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) addStorageInstanceVolume(
	c *tc.C, siUUID, volUUID string,
) {
	_, err := s.DB().Exec(`
INSERT INTO storage_instance_volume (storage_instance_uuid, storage_volume_uuid)
VALUES (?, ?)`, siUUID, volUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) addStorageInstanceFilesystem(
	c *tc.C, siUUID, fsUUID string,
) {
	_, err := s.DB().Exec(`
INSERT INTO storage_instance_filesystem (storage_instance_uuid, storage_filesystem_uuid)
VALUES (?, ?)`, siUUID, fsUUID)
	c.Assert(err, tc.ErrorIsNil)
}
