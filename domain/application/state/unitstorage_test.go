// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/errors"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
	internalerrors "github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// unitStorageSuite is a test suite for asserting state based storage related to
// units.
type unitStorageSuite struct {
	baseSuite
	storageHelper

	state *State
}

// TestUnitStorageSuite registers and runs all of the tests located in the
// [unitStorageSuite].
func TestUnitStorageSuite(t *testing.T) {
	suite := &unitStorageSuite{}
	suite.storageHelper.dbGetter = &suite.ModelSuite
	tc.Run(t, suite)
}

func (u *unitStorageSuite) SetUpTest(c *tc.C) {
	u.baseSuite.SetUpTest(c)

	u.state = NewState(
		u.TxnRunnerFactory(),
		u.modelUUID,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

// newDyingStorageInstanceWithModelFilesystem is a helper function to
// create a new storage instance with life Dying in the model with an
// associated model provisioned filesystem.
func (u *unitStorageSuite) newDyingStorageInstanceWithModelFilesystem(
	c *tc.C,
) (domainstorage.StorageInstanceUUID, domainstorage.FilesystemUUID) {
	return u.newStorageInstanceWithLifeAndWithModelFilesystem(c, life.Dying)
}

// newAliveStorageInstanceWithModelFilesystem is a helper function to
// create a new storage instance with life Dying in the model with an
// associated model provisioned filesystem.
func (u *unitStorageSuite) newAliveStorageInstanceWithModelFilesystem(
	c *tc.C,
) (domainstorage.StorageInstanceUUID, domainstorage.FilesystemUUID) {
	return u.newStorageInstanceWithLifeAndWithModelFilesystem(c, life.Alive)
}

// newStorageInstanceWithModelFilesystem is a helper function to create a new
// storage instance in the model with an associated model provisioned
// filesystem.
func (u *unitStorageSuite) newStorageInstanceWithLifeAndWithModelFilesystem(
	c *tc.C, life life.Life,
) (domainstorage.StorageInstanceUUID, domainstorage.FilesystemUUID) {
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	filesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)

	storagePoolUUID := u.newStoragePool(c, storageInstanceUUID.String(), "test-provider")

	_, err := u.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance (uuid, storage_name, storage_kind_id, storage_id,
                              life_id, storage_pool_uuid, charm_name, requested_size_mib)
VALUES (?, ?, 1, ?, ?, ?, ?, 1024)
`,
		storageInstanceUUID.String(),
		"st1",
		storageInstanceUUID.String(),
		life,
		storagePoolUUID.String(),
		"bar",
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = u.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id, size_mib)
VALUES (?, ?, ?, 0, 1024)
	`,
		filesystemUUID.String(),
		filesystemUUID.String(),
		life,
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = u.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance_filesystem (storage_instance_uuid,
                                         storage_filesystem_uuid)
VALUES (?, ?)
	`,
		storageInstanceUUID.String(),
		filesystemUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return storageInstanceUUID, filesystemUUID
}

// TestGetUnitOwnedStorageInstancesUnitNotFound ensures that calling
// [State.GetUnitOwnedStorageInstances] with a unit uuid that doesn't exists
// returns a [applicationerrors.UnitNotFound] error to the caller.
func (u *unitStorageSuite) TestGetUnitOwnedStorageInstancesUnitNotFound(c *tc.C) {
	unitUUID := tc.Must(c, coreunit.NewUUID)

	_, _, err := u.state.GetUnitOwnedStorageInstances(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetUnitOwnedStorageInstancesNoStorage tests that if the unit has no
// storage that it owns no error is returned and an empty results set is
// provided.
func (u *unitStorageSuite) TestGetUnitOwnedStorageInstancesNoStorage(c *tc.C) {
	_, unitUUIDs := u.createIAASApplicationWithNUnits(c, "foo", life.Alive, 1)
	unitUUID := unitUUIDs[0]

	insts, _, err := u.state.GetUnitOwnedStorageInstances(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(insts, tc.HasLen, 0)
}

func (u *unitStorageSuite) TestGetUnitOwnedStorageInstances(c *tc.C) {
	_, unitUUIDs := u.createIAASApplicationWithNUnits(c, "foo", life.Alive, 1)
	unitUUID := unitUUIDs[0]

	st1UUID, fs1UUID := u.newDyingStorageInstanceWithModelFilesystem(c)
	st2UUID, fs2UUID := u.newDyingStorageInstanceWithModelFilesystem(c)
	u.newStorageUnitOwner(c, st1UUID, unitUUID)
	u.newStorageUnitOwner(c, st2UUID, unitUUID)

	owned, _, err := u.state.GetUnitOwnedStorageInstances(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIsNil)

	expected := []internal.StorageInstanceComposition{
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
				UUID:           fs1UUID,
			},
			UUID: st1UUID,
		},
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
				UUID:           fs2UUID,
			},
			UUID: st2UUID,
		},
	}

	mc := tc.NewMultiChecker()
	mc.AddExpr("_[_].StorageName", tc.Ignore)
	c.Check(owned, mc, expected)
}

func (u *unitStorageSuite) getUnitCharmUUID(c *tc.C, unitUUID coreunit.UUID) string {
	var gotUUID string
	err := u.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT charm_uuid FROM unit WHERE uuid=?", unitUUID).Scan(&gotUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return gotUUID
}

func (u *unitStorageSuite) newUnitWithStorageDirectives(c *tc.C) (coreunit.UUID, domainstorage.StoragePoolUUID) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
		"st2": {
			CountMax:    1,
			CountMin:    1,
			Description: "st2",
			Name:        "st2",
			MinimumSize: 2048,
			Type:        charm.StorageBlock,
		},
		"st3": {
			CountMax:    -1,
			CountMin:    1,
			Description: "st3",
			Name:        "st3",
			MinimumSize: 2048,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(c, "foo", life.Alive, 1, storage)
	unitUUID := unitUUIDs[0]

	charmUUID := u.getUnitCharmUUID(c, unitUUID)

	storagePoolUUID := u.newStoragePool(c, "test-pool", "test-provider")

	_, err := u.DB().ExecContext(
		c.Context(),
		"INSERT INTO unit_storage_directive VALUES (?, ?, ?, ?, ?, ?)",
		unitUUID.String(),
		charmUUID,
		"st1",
		storagePoolUUID.String(),
		5000,
		4,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = u.DB().ExecContext(
		c.Context(),
		"INSERT INTO unit_storage_directive VALUES (?, ?, ?, ?, ?, ?)",
		unitUUID.String(),
		charmUUID,
		"st2",
		storagePoolUUID.String(),
		8000,
		1,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = u.DB().ExecContext(
		c.Context(),
		"INSERT INTO unit_storage_directive VALUES (?, ?, ?, ?, ?, ?)",
		unitUUID.String(),
		charmUUID,
		"st3",
		storagePoolUUID.String(),
		5000,
		8,
	)
	c.Assert(err, tc.ErrorIsNil)
	return unitUUID, storagePoolUUID
}

// TestGetUnitStorageDirectives tests the happy path of getting a units storage
// directives.
func (u *unitStorageSuite) TestGetUnitStorageDirectives(c *tc.C) {
	unitUUID, storagePoolUUID := u.newUnitWithStorageDirectives(c)

	gotDirectives, err := u.state.GetUnitStorageDirectives(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(gotDirectives, tc.SameContents, []application.StorageDirective{
		{
			CharmMetadataName: "foo",
			CharmStorageType:  charm.StorageBlock,
			Count:             1,
			MaxCount:          1,
			Name:              domainstorage.Name("st2"),
			PoolUUID:          storagePoolUUID,
			Size:              8000,
		},
		{
			CharmMetadataName: "foo",
			CharmStorageType:  charm.StorageFilesystem,
			Count:             4,
			MaxCount:          10,
			Name:              domainstorage.Name("st1"),
			PoolUUID:          storagePoolUUID,
			Size:              5000,
		},
		{
			CharmMetadataName: "foo",
			CharmStorageType:  charm.StorageFilesystem,
			Count:             8,
			MaxCount:          -1,
			Name:              domainstorage.Name("st3"),
			PoolUUID:          storagePoolUUID,
			Size:              5000,
		},
	})
}

// TestGetUnitStorageDirectivesEmpty ensures that when a unit has no storage
func (u *unitStorageSuite) TestGetUnitStorageDirectivesEmpty(c *tc.C) {
	_, unitUUIDs := u.createIAASApplicationWithNUnits(c, "foo", life.Alive, 1)
	unitUUID := unitUUIDs[0]

	directives, err := u.state.GetUnitStorageDirectives(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(directives, tc.HasLen, 0)
}

// TestGetUnitStorageDirectivesUnitNotFound ensures that when asking for the
// storage directives of a unit that does not exist in the model the caller gets
// back a [applicationerrors.UnitNotFound] error.
func (u *unitStorageSuite) TestGetUnitStorageDirectivesUnitNotFound(c *tc.C) {
	unitUUID := tc.Must(c, coreunit.NewUUID)
	_, err := u.state.GetUnitStorageDirectives(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (u *unitStorageSuite) TestGetUnitStorageDirectiveByNameUnitNotFound(c *tc.C) {
	unitUUID := tc.Must(c, coreunit.NewUUID)
	_, err := u.state.GetUnitStorageDirectiveByName(c.Context(), unitUUID, "pgdata")
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (u *unitStorageSuite) TestGetUnitStorageDirectiveByNameNotSupported(c *tc.C) {
	_, unitUUIDs := u.createIAASApplicationWithNUnits(c, "foo", life.Alive, 1)
	unitUUID := unitUUIDs[0]

	_, err := u.state.GetUnitStorageDirectiveByName(c.Context(), unitUUID, "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.StorageNameNotSupported)
}

func (u *unitStorageSuite) TestGetUnitStorageDirectiveByName(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
		"st2": {
			CountMax:    1,
			CountMin:    1,
			Description: "st2",
			Name:        "st2",
			MinimumSize: 2048,
			Type:        charm.StorageBlock,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(c, "foo", life.Alive, 1, storage)
	unitUUID := unitUUIDs[0]

	charmUUID := u.getUnitCharmUUID(c, unitUUID)

	storagePoolUUID := u.newStoragePool(c, "test-pool", "test-provider")

	_, err := u.DB().ExecContext(
		c.Context(),
		"INSERT INTO unit_storage_directive VALUES (?, ?, ?, ?, ?, ?)",
		unitUUID.String(),
		charmUUID,
		"st1",
		storagePoolUUID.String(),
		5000,
		4,
	)
	c.Assert(err, tc.ErrorIsNil)
	_, err = u.DB().ExecContext(
		c.Context(),
		"INSERT INTO unit_storage_directive VALUES (?, ?, ?, ?, ?, ?)",
		unitUUID.String(),
		charmUUID,
		"st2",
		storagePoolUUID.String(),
		8000,
		1,
	)
	c.Assert(err, tc.ErrorIsNil)

	gotDirective, err := u.state.GetUnitStorageDirectiveByName(c.Context(), unitUUID, "st2")
	c.Check(err, tc.ErrorIsNil)
	c.Assert(gotDirective, tc.DeepEquals, application.StorageDirective{
		CharmMetadataName: "foo",
		CharmStorageType:  charm.StorageBlock,
		Count:             1,
		MaxCount:          1,
		Name:              "st2",
		PoolUUID:          storagePoolUUID,
		Size:              8000,
	})
}

func (u *unitStorageSuite) TestGetStorageAddInfoByUnitUUID(c *tc.C) {
	unitUUID, _ := u.newUnitWithStorageDirectives(c)

	st1UUID, _ := u.newDyingStorageInstanceWithModelFilesystem(c)
	st2UUID, _ := u.newDyingStorageInstanceWithModelFilesystem(c)
	u.newStorageUnitOwner(c, st1UUID, unitUUID)
	u.newStorageUnitOwner(c, st2UUID, unitUUID)

	storageInfo, err := u.state.GetStorageAddInfoByUnitUUID(c.Context(), unitUUID, "st1")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageInfo, tc.DeepEquals, internal.StorageInfoForAdd{
		CharmStorageName:     "st1",
		Type:                 "filesystem",
		CountMin:             1,
		CountMax:             10,
		MinimumSize:          1024,
		AlreadyAttachedCount: uint32(2),
	})
}

func (u *unitStorageSuite) TestGetStorageAddInfoByUnitUUIDNotSupported(c *tc.C) {
	unitUUID, _ := u.newUnitWithStorageDirectives(c)

	_, err := u.state.GetStorageAddInfoByUnitUUID(c.Context(), unitUUID, "st666")
	c.Assert(err, tc.ErrorIs, applicationerrors.StorageNameNotSupported)
}

func (u *unitStorageSuite) TestAddStorageForIAASUnitNotFound(c *tc.C) {
	uuid := tc.Must(c, coreunit.NewUUID)
	_, err := u.state.AddStorageForIAASUnit(c.Context(), uuid, "st1", internal.AddStorageToIAASUnitArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (u *unitStorageSuite) TestAddStorageForIAASUnitNotAlive(c *tc.C) {
	_, uuid := u.createNamedIAASUnit(c)

	err := u.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE uuid = ?", uuid.String())
		return err
	})
	c.Assert(err, tc.IsNil)

	_, err = u.state.AddStorageForIAASUnit(c.Context(), uuid, "st1", internal.AddStorageToIAASUnitArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotAlive)
}

func (u *unitStorageSuite) TestAddStorageForIAASUnit(c *tc.C) {
	unitUUID, poolUUID := u.newUnitWithStorageDirectives(c)
	netNodeUUID, err := u.state.GetUnitNetNodeUUID(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	fs1UUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	si1UUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	fs2UUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	si2UUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	unitStorageToCreate := []internal.CreateUnitStorageInstanceArg{
		{
			Filesystem: &internal.CreateUnitStorageFilesystemArg{
				UUID: fs1UUID,
			},
			Name:            "st1",
			UUID:            si1UUID,
			Kind:            domainstorage.StorageKindFilesystem,
			StoragePoolUUID: poolUUID,
			RequestSizeMiB:  1024,
		},
		{
			Filesystem: &internal.CreateUnitStorageFilesystemArg{
				UUID: fs2UUID,
			},
			Name:            "st2",
			UUID:            si2UUID,
			Kind:            domainstorage.StorageKindFilesystem,
			StoragePoolUUID: poolUUID,
			RequestSizeMiB:  256,
		},
	}

	sa1UUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	fsa1UUID := tc.Must(c, domainstorage.NewFilesystemAttachmentUUID)
	sa2UUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	fsa2UUID := tc.Must(c, domainstorage.NewFilesystemAttachmentUUID)
	unitStorageToAttach := []internal.CreateUnitStorageAttachmentArg{
		{
			UUID: sa1UUID,
			FilesystemAttachment: &internal.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: fs1UUID,
				NetNodeUUID:    domainnetwork.NetNodeUUID(netNodeUUID),
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
				UUID:           fsa1UUID,
			},
			StorageInstanceUUID: si1UUID,
		}, {
			UUID: sa2UUID,
			FilesystemAttachment: &internal.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: fs2UUID,
				NetNodeUUID:    domainnetwork.NetNodeUUID(netNodeUUID),
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
				UUID:           fsa2UUID,
			},
			StorageInstanceUUID: si2UUID,
		},
	}

	gotIDs, err := u.state.AddStorageForIAASUnit(c.Context(), unitUUID, "st1", internal.AddStorageToIAASUnitArg{
		AddStorageToUnitArg: internal.AddStorageToUnitArg{
			StorageInstances: unitStorageToCreate,
			StorageToAttach:  unitStorageToAttach,
			StorageToOwn:     []domainstorage.StorageInstanceUUID{si1UUID, si2UUID},
		},
		FilesystemsToOwn: []domainstorage.FilesystemUUID{fs1UUID, fs2UUID},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotIDs, tc.SameContents, []corestorage.ID{"st1/0", "st2/1"})

	inst, attach, err := u.state.GetUnitOwnedStorageInstances(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(inst, tc.SameContents, []internal.StorageInstanceComposition{
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProvisionScope: 0,
				UUID:           fs1UUID,
			},
			StorageName: "st1",
			UUID:        si1UUID,
		}, {
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProvisionScope: 0,
				UUID:           fs2UUID,
			},
			StorageName: "st2",
			UUID:        si2UUID,
		},
	})
	c.Assert(attach, tc.SameContents, []internal.StorageAttachmentComposition{
		{
			UUID:                sa1UUID,
			StorageInstanceUUID: si1UUID,
			FilesystemAttachment: &internal.StorageInstanceCompositionFilesystemAttachment{
				ProvisionScope: 1,
				UUID:           fsa1UUID,
				FilesystemUUID: fs1UUID,
			},
		}, {
			UUID:                sa2UUID,
			StorageInstanceUUID: si2UUID,
			FilesystemAttachment: &internal.StorageInstanceCompositionFilesystemAttachment{
				ProvisionScope: 0,
				UUID:           fsa2UUID,
				FilesystemUUID: fs2UUID,
			},
		},
	})
}

func (u *unitStorageSuite) TestAttachStorageToIAASUnitNotFound(c *tc.C) {
	unitUUID := tc.Must(c, coreunit.NewUUID)
	stUUID, _ := u.newDyingStorageInstanceWithModelFilesystem(c)
	err := u.state.AttachStorageToUnit(c.Context(), stUUID, unitUUID, internal.AttachStorageToUnitArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (u *unitStorageSuite) TestAttachStorageToIAASUnitStorageNotFound(c *tc.C) {
	_, unitUUID := u.createNamedIAASUnit(c)
	storageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	err := u.state.AttachStorageToUnit(c.Context(), storageUUID, unitUUID, internal.AttachStorageToUnitArg{})
	c.Assert(err, tc.ErrorIs, errors.StorageInstanceNotFound)
}

func (u *unitStorageSuite) TestAttachStorageToIAASUnitNotAlive(c *tc.C) {
	_, unitUUID := u.createNamedIAASUnit(c)
	stUUID, _ := u.newAliveStorageInstanceWithModelFilesystem(c)

	err := u.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE uuid = ?", unitUUID.String())
		return err
	})
	c.Assert(err, tc.IsNil)

	err = u.state.AttachStorageToUnit(c.Context(), stUUID, unitUUID, internal.AttachStorageToUnitArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotAlive)
}

func (u *unitStorageSuite) TestAttachStorageToIAASUnitStorageNotAlive(c *tc.C) {
	_, unitUUID := u.createNamedIAASUnit(c)
	stUUID, _ := u.newDyingStorageInstanceWithModelFilesystem(c)

	err := u.state.AttachStorageToUnit(c.Context(), stUUID, unitUUID, internal.AttachStorageToUnitArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.StorageNotAlive)
}

func (u *unitStorageSuite) TestAttachStorageToUnit(c *tc.C) {
	unitUUID, _ := u.newUnitWithStorageDirectives(c)
	netNodeUUID, err := u.state.GetUnitNetNodeUUID(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	siUUID, fsUUID := u.newAliveStorageInstanceWithModelFilesystem(c)
	saUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	fsaUUID := tc.Must(c, domainstorage.NewFilesystemAttachmentUUID)

	unitStorageToAttach := internal.CreateUnitStorageAttachmentArg{
		UUID: saUUID,
		FilesystemAttachment: &internal.CreateUnitStorageFilesystemAttachmentArg{
			FilesystemUUID: fsUUID,
			NetNodeUUID:    domainnetwork.NetNodeUUID(netNodeUUID),
			ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			UUID:           fsaUUID,
		},
		StorageInstanceUUID: siUUID,
	}

	attachInfo, err := u.state.GetStorageAttachInfoByUnitUUIDAndStorageUUID(c.Context(), unitUUID, siUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(attachInfo.AlreadyAttachedToUnits, tc.HasLen, 0)

	err = u.state.AttachStorageToUnit(c.Context(), siUUID, unitUUID, internal.AttachStorageToUnitArg{
		StorageToAttach: unitStorageToAttach,
	})
	c.Assert(err, tc.ErrorIsNil)

	var siCharmName string
	err = u.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT charm_name FROM storage_instance WHERE uuid=?", siUUID).Scan(&siCharmName)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(siCharmName, tc.Equals, "foo")

	inst, attach, err := u.state.GetUnitOwnedStorageInstances(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(inst, tc.SameContents, []internal.StorageInstanceComposition{
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProvisionScope: 0,
				UUID:           fsUUID,
			},
			StorageName: "st1",
			UUID:        siUUID,
		},
	})
	c.Assert(attach, tc.SameContents, []internal.StorageAttachmentComposition{
		{
			UUID:                saUUID,
			StorageInstanceUUID: siUUID,
			FilesystemAttachment: &internal.StorageInstanceCompositionFilesystemAttachment{
				ProvisionScope: 1,
				UUID:           fsaUUID,
				FilesystemUUID: fsUUID,
			},
		},
	})

	attachInfo, err = u.state.GetStorageAttachInfoByUnitUUIDAndStorageUUID(c.Context(), unitUUID, siUUID)
	c.Assert(err, tc.IsNil)
	c.Assert(attachInfo.AlreadyAttachedToUnits, tc.DeepEquals, map[string]string{unitUUID.String(): "foo/0"})
}

func (u *unitStorageSuite) TestAttachStorageAlreadyAttached(c *tc.C) {
	unitUUID, _ := u.newUnitWithStorageDirectives(c)
	netNodeUUID, err := u.state.GetUnitNetNodeUUID(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	siUUID, fsUUID := u.newAliveStorageInstanceWithModelFilesystem(c)
	saUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	fsaUUID := tc.Must(c, domainstorage.NewFilesystemAttachmentUUID)

	unitStorageToAttach := internal.CreateUnitStorageAttachmentArg{
		UUID: saUUID,
		FilesystemAttachment: &internal.CreateUnitStorageFilesystemAttachmentArg{
			FilesystemUUID: fsUUID,
			NetNodeUUID:    domainnetwork.NetNodeUUID(netNodeUUID),
			ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			UUID:           fsaUUID,
		},
		StorageInstanceUUID: siUUID,
	}

	arg := internal.AttachStorageToUnitArg{
		StorageToAttach: unitStorageToAttach,
	}
	err = u.state.AttachStorageToUnit(c.Context(), siUUID, unitUUID, arg)
	c.Assert(err, tc.ErrorIsNil)

	arg.AllowedExistingUnitAttachments = []string{tc.Must(c, coreunit.NewUUID).String()}
	err = u.state.AttachStorageToUnit(c.Context(), siUUID, unitUUID, arg)
	rErr, ok := internalerrors.AsType[applicationerrors.StorageAttachmentNotAllowed](err)
	c.Assert(ok, tc.IsTrue)
	c.Assert(rErr, tc.DeepEquals, applicationerrors.StorageAttachmentNotAllowed{
		AttachedToUnits: []string{"foo/0"},
	})
}

func (u *unitStorageSuite) TestAttachStorageTwiceSameUnit(c *tc.C) {
	unitUUID, _ := u.newUnitWithStorageDirectives(c)
	netNodeUUID, err := u.state.GetUnitNetNodeUUID(c.Context(), unitUUID)
	c.Assert(err, tc.ErrorIsNil)

	siUUID, fsUUID := u.newAliveStorageInstanceWithModelFilesystem(c)
	saUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	fsaUUID := tc.Must(c, domainstorage.NewFilesystemAttachmentUUID)

	unitStorageToAttach := internal.CreateUnitStorageAttachmentArg{
		UUID: saUUID,
		FilesystemAttachment: &internal.CreateUnitStorageFilesystemAttachmentArg{
			FilesystemUUID: fsUUID,
			NetNodeUUID:    domainnetwork.NetNodeUUID(netNodeUUID),
			ProvisionScope: domainstorageprov.ProvisionScopeMachine,
			UUID:           fsaUUID,
		},
		StorageInstanceUUID: siUUID,
	}

	arg := internal.AttachStorageToUnitArg{
		StorageToAttach: unitStorageToAttach,
	}
	err = u.state.AttachStorageToUnit(c.Context(), siUUID, unitUUID, arg)
	c.Assert(err, tc.ErrorIsNil)

	arg.AllowedExistingUnitAttachments = []string{unitUUID.String()}
	arg.CountLessThanEqual = 2
	err = u.state.AttachStorageToUnit(c.Context(), siUUID, unitUUID, arg)
	c.Assert(err, tc.ErrorIsNil)
}

func (u *unitStorageSuite) TestGetStorageInstanceCompositionByUUIDNotFound(c *tc.C) {
	uuid := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	_, err := u.state.GetStorageInstanceCompositionByUUID(c.Context(), uuid)
	c.Assert(err, tc.ErrorIs, errors.StorageInstanceNotFound)
}

func (u *unitStorageSuite) TestGetStorageInstanceCompositionByUUID(c *tc.C) {
	_, unitUUIDs := u.createIAASApplicationWithNUnits(c, "foo", life.Alive, 1)
	unitUUID := unitUUIDs[0]

	st1UUID, fs1UUID := u.newDyingStorageInstanceWithModelFilesystem(c)
	st2UUID, _ := u.newDyingStorageInstanceWithModelFilesystem(c)
	u.newStorageUnitOwner(c, st1UUID, unitUUID)
	u.newStorageUnitOwner(c, st2UUID, unitUUID)

	result, err := u.state.GetStorageInstanceCompositionByUUID(c.Context(), st1UUID)
	c.Assert(err, tc.ErrorIsNil)

	expected := internal.StorageInstanceComposition{
		Filesystem: &internal.StorageInstanceCompositionFilesystem{
			ProvisionScope: domainstorageprov.ProvisionScopeModel,
			UUID:           fs1UUID,
		},
		StorageName: "st1",
		UUID:        st1UUID,
	}

	mc := tc.NewMultiChecker()
	mc.AddExpr("_[_].StorageName", tc.Ignore)
	c.Check(result, mc, expected)
}

func (u *unitStorageSuite) TestGetStorageAttachInfoByUnitUUIDAndStorageUUIDNotFound(c *tc.C) {
	unitUUID, _ := u.newUnitWithStorageDirectives(c)
	stUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	_, err := u.state.GetStorageAttachInfoByUnitUUIDAndStorageUUID(c.Context(), unitUUID, stUUID)
	c.Assert(err, tc.ErrorIs, errors.StorageInstanceNotFound)
}

func (u *unitStorageSuite) TestGetStorageAttachInfoByUnitUUIDAndStorageUUID(c *tc.C) {
	unitUUID, _ := u.newUnitWithStorageDirectives(c)

	st1UUID, _ := u.newDyingStorageInstanceWithModelFilesystem(c)
	st2UUID, _ := u.newDyingStorageInstanceWithModelFilesystem(c)
	u.newStorageUnitOwner(c, st1UUID, unitUUID)
	u.newStorageUnitOwner(c, st2UUID, unitUUID)

	storageInfo, err := u.state.GetStorageAttachInfoByUnitUUIDAndStorageUUID(c.Context(), unitUUID, st1UUID)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(storageInfo, tc.DeepEquals, internal.StorageInfoForAttach{
		CharmStorageName:       "st1",
		CountMin:               1,
		CountMax:               10,
		MinimumSize:            1024,
		AlreadyAttachedCount:   2,
		ProvisionedSizeMiB:     1024,
		AlreadyAttachedToUnits: nil,
	})
}
