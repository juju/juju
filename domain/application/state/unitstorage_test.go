// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	coremachine "github.com/juju/juju/core/machine"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
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
				ProvisionScope: domainstorage.ProvisionScopeModel,
				UUID:           fs1UUID,
			},
			UUID: st1UUID,
		},
		{
			Filesystem: &internal.StorageInstanceCompositionFilesystem{
				ProvisionScope: domainstorage.ProvisionScopeModel,
				UUID:           fs2UUID,
			},
			UUID: st2UUID,
		},
	}

	mc := tc.NewMultiChecker()
	mc.AddExpr("_[_].StorageName", tc.Ignore)
	c.Check(owned, mc, expected)
}

func (u *unitStorageSuite) getUnitCharmUUID(c *tc.C, unitUUID coreunit.UUID) corecharm.ID {
	var gotUUID string
	err := u.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT charm_uuid FROM unit WHERE uuid=?", unitUUID).Scan(&gotUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	charmID, err := corecharm.ParseID(gotUUID)
	c.Assert(err, tc.ErrorIsNil)
	return charmID
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
	otherCharmUUID, _, err := u.state.AddCharm(c.Context(), charm.Charm{
		Metadata: charm.Metadata{
			Name:    "another",
			Storage: storage,
		},
		Manifest: charm.Manifest{
			Bases: []charm.Base{
				{
					Name: "ubuntu",
					Channel: charm.Channel{
						Risk: charm.RiskStable,
					},
					Architectures: []string{"amd64"},
				},
			},
		},
		ReferenceName: "another",
		Source:        charm.CharmHubSource,
		Revision:      42,
		Hash:          "hash",
	},
		&charm.DownloadInfo{
			Provenance:         charm.ProvenanceDownload,
			CharmhubIdentifier: "ident",
			DownloadURL:        "https://example.com",
			DownloadSize:       42,
		},
		false,
	)
	c.Assert(err, tc.ErrorIsNil)

	storagePoolUUID := u.newStoragePool(c, "test-pool", "test-provider")

	_, err = u.DB().ExecContext(
		c.Context(),
		"INSERT INTO unit_storage_directive VALUES (?, ?, ?, ?, ?, ?)",
		unitUUID.String(),
		charmUUID.String(),
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
		charmUUID.String(),
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
		charmUUID.String(),
		"st3",
		storagePoolUUID.String(),
		5000,
		8,
	)
	c.Assert(err, tc.ErrorIsNil)
	// Other charm with same unit.
	_, err = u.DB().ExecContext(
		c.Context(),
		"INSERT INTO unit_storage_directive VALUES (?, ?, ?, ?, ?, ?)",
		unitUUID.String(),
		otherCharmUUID,
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
	c.Check(gotDirectives, tc.SameContents, []internal.StorageDirective{
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
	otherCharmUUID, _, err := u.state.AddCharm(c.Context(), charm.Charm{
		Metadata: charm.Metadata{
			Name:    "another",
			Storage: storage,
		},
		Manifest: charm.Manifest{
			Bases: []charm.Base{
				{
					Name: "ubuntu",
					Channel: charm.Channel{
						Risk: charm.RiskStable,
					},
					Architectures: []string{"amd64"},
				},
			},
		},
		ReferenceName: "another",
		Source:        charm.CharmHubSource,
		Revision:      42,
		Hash:          "hash",
	},
		&charm.DownloadInfo{
			Provenance:         charm.ProvenanceDownload,
			CharmhubIdentifier: "ident",
			DownloadURL:        "https://example.com",
			DownloadSize:       42,
		},
		false,
	)
	c.Assert(err, tc.ErrorIsNil)

	storagePoolUUID := u.newStoragePool(c, "test-pool", "test-provider")

	_, err = u.DB().ExecContext(
		c.Context(),
		"INSERT INTO unit_storage_directive VALUES (?, ?, ?, ?, ?, ?)",
		unitUUID.String(),
		charmUUID.String(),
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
		charmUUID.String(),
		"st2",
		storagePoolUUID.String(),
		8000,
		1,
	)
	c.Assert(err, tc.ErrorIsNil)
	// Insert a unit storage directive for the same unit, wrong charm.
	_, err = u.DB().ExecContext(
		c.Context(),
		"INSERT INTO unit_storage_directive VALUES (?, ?, ?, ?, ?, ?)",
		unitUUID.String(),
		otherCharmUUID,
		"st2",
		storagePoolUUID.String(),
		8000,
		1,
	)
	c.Assert(err, tc.ErrorIsNil)

	gotDirective, err := u.state.GetUnitStorageDirectiveByName(c.Context(), unitUUID, "st2")
	c.Check(err, tc.ErrorIsNil)
	c.Assert(gotDirective, tc.DeepEquals, internal.StorageDirective{
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
		CharmStorageDefinitionForValidation: internal.CharmStorageDefinitionForValidation{
			Name:        "st1",
			Type:        "filesystem",
			CountMin:    1,
			CountMax:    10,
			MinimumSize: 1024,
		},
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
	_, err := u.state.AddStorageForIAASUnit(c.Context(), uuid, "st1", domainstorage.IAASUnitAddStorageArg{})
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (u *unitStorageSuite) TestAddStorageForIAASUnitNotAlive(c *tc.C) {
	_, uuid := u.createNamedIAASUnit(c)

	err := u.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE unit SET life_id = 1 WHERE uuid = ?", uuid.String())
		return err
	})
	c.Assert(err, tc.IsNil)

	_, err = u.state.AddStorageForIAASUnit(c.Context(), uuid, "st1", domainstorage.IAASUnitAddStorageArg{})
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
	unitStorageToCreate := []domainstorage.CreateUnitStorageInstanceArg{
		{
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
				UUID: fs1UUID,
			},
			Name:            "st1",
			UUID:            si1UUID,
			Kind:            domainstorage.StorageKindFilesystem,
			StoragePoolUUID: poolUUID,
			RequestSizeMiB:  1024,
		},
		{
			Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
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
	unitStorageToAttach := []internal.CreateStorageInstanceAttachmentArg{
		{
			UUID: sa1UUID,
			FilesystemAttachment: &domainstorage.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: fs1UUID,
				NetNodeUUID:    domainnetwork.NetNodeUUID(netNodeUUID),
				ProvisionScope: domainstorage.ProvisionScopeMachine,
				UUID:           fsa1UUID,
			},
			StorageInstanceUUID: si1UUID,
		}, {
			UUID: sa2UUID,
			FilesystemAttachment: &domainstorage.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: fs2UUID,
				NetNodeUUID:    domainnetwork.NetNodeUUID(netNodeUUID),
				ProvisionScope: domainstorage.ProvisionScopeModel,
				UUID:           fsa2UUID,
			},
			StorageInstanceUUID: si2UUID,
		},
	}

	gotIDs, err := u.state.AddStorageForIAASUnit(c.Context(), unitUUID, "st1", domainstorage.IAASUnitAddStorageArg{
		UnitAddStorageArg: domainstorage.UnitAddStorageArg{
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

// TestAttachStorageToUnitStorageInstanceNotFound verifies that attaching
// storage to a unitfor a missing Storage Instance returns an error satisfying
// [domainstorageerrors.StorageInstanceNotFound].
func (u *unitStorageSuite) TestAttachStorageToUnitStorageInstanceNotFound(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 1, storage,
	)
	unitUUID := unitUUIDs[0]

	storageInstUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	err := u.state.AttachStorageToUnit(
		c.Context(),
		unitUUID,
		internal.AttachStorageInstanceToUnitArg{
			CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
				StorageInstanceUUID: storageInstUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestAttachStorageToUnitStorageInstanceNotAlive verifies that attaching
// storage for a dying Storage Instance returns an error satisfying
// [domainstorageerrors.StorageInstanceNotAlive].
func (u *unitStorageSuite) TestAttachStorageToUnitStorageInstanceNotAlive(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 1, storage,
	)
	unitUUID := unitUUIDs[0]

	storageInstUUID, _ := u.newDyingStorageInstanceWithModelFilesystem(c)

	err := u.state.AttachStorageToUnit(
		c.Context(),
		unitUUID,
		internal.AttachStorageInstanceToUnitArg{
			CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
				StorageInstanceUUID: storageInstUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotAlive)
}

// TestAttachStorageToUnitUnitNotFound verifies that attaching storage for a
// missing unit returns an error satisfying [applicationerrors.UnitNotFound].
func (u *unitStorageSuite) TestAttachStorageToUnitUnitNotFound(c *tc.C) {
	storageInstUUID, _ := u.newAliveStorageInstanceWithModelFilesystem(c)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	err := u.state.AttachStorageToUnit(
		c.Context(),
		unitUUID,
		internal.AttachStorageInstanceToUnitArg{
			CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
				StorageInstanceUUID: storageInstUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestAttachStorageToUnitUnitNotAlive verifies that attaching storage for a
// dying unit returns an error satisfying [applicationerrors.UnitNotAlive].
func (u *unitStorageSuite) TestAttachStorageToUnitUnitNotAlive(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 1, storage,
	)
	unitUUID := unitUUIDs[0]
	u.setUnitLife(c, unitUUID, life.Dying)

	storageInstUUID, _ := u.newAliveStorageInstanceWithModelFilesystem(c)

	err := u.state.AttachStorageToUnit(
		c.Context(),
		unitUUID,
		internal.AttachStorageInstanceToUnitArg{
			CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
				StorageInstanceUUID: storageInstUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotAlive)
}

// TestAttachStorageToUnitStorageInstanceAlreadyAttached verifies that
// attaching storage when the storage instance is already attached to the unit
// returns an error satisfying
// [applicationerrors.StorageInstanceAlreadyAttachedToUnit].
func (u *unitStorageSuite) TestAttachStorageToUnitStorageInstanceAlreadyAttached(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 1, storage,
	)
	unitUUID := unitUUIDs[0]

	unitCharmUUID := u.getUnitCharmUUID(c, unitUUID)
	unitMachineUUID := u.getUnitMachineUUID(c, unitUUID)
	storageInstUUID := u.newStorageInstanceWithName(c, "st1")
	u.newStorageInstanceAttachment(c, storageInstUUID, unitUUID)

	unitStorageToAttach := internal.CreateStorageInstanceAttachmentArg{
		StorageInstanceUUID: storageInstUUID,
	}

	attachArgs := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: unitStorageToAttach,
		UnitStorageInstanceAttachmentCheckArgs: internal.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: 0,
			CharmUUID:          unitCharmUUID,
			MachineUUID:        &unitMachineUUID,
		},
	}

	err := u.state.AttachStorageToUnit(c.Context(), unitUUID, attachArgs)
	c.Check(err, tc.ErrorIs, applicationerrors.StorageInstanceAlreadyAttachedToUnit)
}

// TestAttachStorageToUnitAttachmentCountExceedsLimit verifies that attaching
// storage when the unit already has too many attachments returns an error
// satisfying [applicationerrors.UnitAttachmentCountExceedsLimit].
func (u *unitStorageSuite) TestAttachStorageToUnitAttachmentCountExceedsLimit(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    2,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 1, storage,
	)
	unitUUID := unitUUIDs[0]

	unitCharmUUID := u.getUnitCharmUUID(c, unitUUID)
	unitMachineUUID := u.getUnitMachineUUID(c, unitUUID)

	storageInst1UUID := u.newStorageInstanceWithName(c, "st1")
	storageInst2UUID := u.newStorageInstanceWithName(c, "st1")
	u.newStorageInstanceAttachment(c, storageInst1UUID, unitUUID)
	u.newStorageInstanceAttachment(c, storageInst2UUID, unitUUID)

	storageInst3UUID := u.newStorageInstanceWithName(c, "st1")
	unitStorageToAttach := internal.CreateStorageInstanceAttachmentArg{
		StorageInstanceUUID: storageInst3UUID,
	}

	attachArgs := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: unitStorageToAttach,
		UnitStorageInstanceAttachmentCheckArgs: internal.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: 1,
			CharmUUID:          unitCharmUUID,
			MachineUUID:        &unitMachineUUID,
		},
	}

	err := u.state.AttachStorageToUnit(c.Context(), unitUUID, attachArgs)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitAttachmentCountExceedsLimit)
}

// TestAttachStorageToUnitUnitCharmChanged verifies that attaching storage with
// a mismatched charm returns an error satisfying
// [applicationerrors.UnitCharmChanged].
func (u *unitStorageSuite) TestAttachStorageToUnitUnitCharmChanged(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 1, storage,
	)
	unitUUID := unitUUIDs[0]

	unitMachineUUID := u.getUnitMachineUUID(c, unitUUID)
	storageInstUUID := u.newStorageInstanceWithName(c, "st1")
	otherCharmUUID := tc.Must(c, corecharm.NewID)

	attachArgs := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
			StorageInstanceUUID: storageInstUUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: internal.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: 0,
			CharmUUID:          otherCharmUUID,
			MachineUUID:        &unitMachineUUID,
		},
	}

	err := u.state.AttachStorageToUnit(c.Context(), unitUUID, attachArgs)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitCharmChanged)
}

// TestAttachStorageToUnitUnitMachineChanged verifies that attaching storage
// with a mismatched machine returns an error satisfying
// [applicationerrors.UnitMachineChanged].
func (u *unitStorageSuite) TestAttachStorageToUnitUnitMachineChanged(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 1, storage,
	)
	unitUUID := unitUUIDs[0]

	unitCharmUUID := u.getUnitCharmUUID(c, unitUUID)
	otherMachineUUID := tc.Must(c, coremachine.NewUUID)
	storageInstUUID := u.newStorageInstanceWithName(c, "st1")

	attachArgs := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
			StorageInstanceUUID: storageInstUUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: internal.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: 0,
			CharmUUID:          unitCharmUUID,
			MachineUUID:        &otherMachineUUID,
		},
	}

	err := u.state.AttachStorageToUnit(c.Context(), unitUUID, attachArgs)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitMachineChanged)
}

// TestAttachStorageToUnitStorageInstanceUnexpectedAttachments verifies that
// attaching storage with unexpected existing attachments returns an error
// satisfying [applicationerrors.StorageInstanceUnexpectedAttachments].
//
// This test checks that the Storage Instance is attached to one unit but in
// reality two attachments exists violating the pre-calculated expectations.
func (u *unitStorageSuite) TestAttachStorageToUnitStorageInstanceUnexpectedAttachments(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 3, storage,
	)
	unitUUID1 := unitUUIDs[0]
	unitUUID2 := unitUUIDs[1]
	unitUUID3 := unitUUIDs[2]

	unitCharmUUID := u.getUnitCharmUUID(c, unitUUID1)
	unitMachineUUID := u.getUnitMachineUUID(c, unitUUID1)
	storageInstUUID := u.newStorageInstanceWithName(c, "st1")
	saUUID1 := u.newStorageInstanceAttachment(c, storageInstUUID, unitUUID2)
	u.newStorageInstanceAttachment(c, storageInstUUID, unitUUID3)

	attachArgs := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
			StorageInstanceUUID: storageInstUUID,
		},
		StorageInstanceAttachmentCheckArgs: internal.StorageInstanceAttachmentCheckArgs{
			ExpectedAttachments: []domainstorage.StorageAttachmentUUID{saUUID1},
			UUID:                storageInstUUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: internal.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: 0,
			CharmUUID:          unitCharmUUID,
			MachineUUID:        &unitMachineUUID,
		},
	}

	err := u.state.AttachStorageToUnit(c.Context(), unitUUID1, attachArgs)
	c.Assert(err, tc.ErrorIs, applicationerrors.StorageInstanceUnexpectedAttachments)
}

// TestAttachStorageToUnitStorageInstanceNoExpectedAttachments verifies that
// attaching storage without listing existing attachments returns an error
// satisfying [applicationerrors.StorageInstanceUnexpectedAttachments].
func (u *unitStorageSuite) TestAttachStorageToUnitStorageInstanceNoExpectedAttachments(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 2, storage,
	)
	unitUUID1 := unitUUIDs[0]
	unitUUID2 := unitUUIDs[1]

	unitCharmUUID := u.getUnitCharmUUID(c, unitUUID1)
	unitMachineUUID := u.getUnitMachineUUID(c, unitUUID1)
	storageInstUUID := u.newStorageInstanceWithName(c, "st1")
	u.newStorageInstanceAttachment(c, storageInstUUID, unitUUID2)

	attachArgs := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
			StorageInstanceUUID: storageInstUUID,
		},
		StorageInstanceAttachmentCheckArgs: internal.StorageInstanceAttachmentCheckArgs{
			UUID: storageInstUUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: internal.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: 0,
			CharmUUID:          unitCharmUUID,
			MachineUUID:        &unitMachineUUID,
		},
	}

	err := u.state.AttachStorageToUnit(c.Context(), unitUUID1, attachArgs)
	c.Assert(err, tc.ErrorIs, applicationerrors.StorageInstanceUnexpectedAttachments)
}

// TestAttachStorageToUnitNoExistingAttachments verifies that attaching a
// Storage Instance with no existing attachments succeeds.
func (u *unitStorageSuite) TestAttachStorageToUnitNoExistingAttachments(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 1, storage,
	)
	unitUUID := unitUUIDs[0]

	unitCharmUUID := u.getUnitCharmUUID(c, unitUUID)
	unitMachineUUID := u.getUnitMachineUUID(c, unitUUID)
	unitMachineNetNodeUUID := u.getMachineNetNodeUUID(c, unitMachineUUID)
	storageInstUUID := u.newStorageInstanceWithName(c, "st1")
	storageInstUUID, filessystemUUID := u.newModelFilesystemStorageInstance(c, "st1", unitCharmUUID)
	storageInstAttachUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	filesystemAttachUUID := tc.Must(c, domainstorage.NewFilesystemAttachmentUUID)

	attachArgs := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
			FilesystemAttachment: &internal.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: filessystemUUID,
				NetNodeUUID:    unitMachineNetNodeUUID,
				ProvisionScope: domainstorageprov.ProvisionScopeModel,
				UUID:           filesystemAttachUUID,
			},
			StorageInstanceUUID: storageInstUUID,
			UUID:                storageInstAttachUUID,
		},
		StorageInstanceAttachmentCheckArgs: internal.StorageInstanceAttachmentCheckArgs{
			UUID: storageInstUUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: internal.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: 0,
			CharmUUID:          unitCharmUUID,
			MachineUUID:        &unitMachineUUID,
		},
	}

	err := u.state.AttachStorageToUnit(c.Context(), unitUUID, attachArgs)
	c.Assert(err, tc.ErrorIsNil)

	u.assertStorageInstanceAttachmentExists(
		c,
		storageInstAttachUUID,
		storageInstUUID,
		unitUUID,
	)
	u.assertFilesystemAttachmentExists(c, filesystemAttachUUID)
}

// TestAttachStorageToUnitSetsCharmName verifies that attaching storage with a
// charm name set argument updates the storage instance charm name.
func (u *unitStorageSuite) TestAttachStorageToUnitSetsCharmName(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 1, storage,
	)
	unitUUID := unitUUIDs[0]

	unitCharmUUID := u.getUnitCharmUUID(c, unitUUID)
	unitMachineUUID := u.getUnitMachineUUID(c, unitUUID)
	storageInstUUID := u.newStorageInstanceWithName(c, "st1")
	attachUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)

	attachArgs := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
			StorageInstanceUUID: storageInstUUID,
			UUID:                attachUUID,
		},
		StorageInstanceAttachmentCheckArgs: internal.StorageInstanceAttachmentCheckArgs{
			UUID: storageInstUUID,
		},
		StorageInstanceCharmNameSetArg: &internal.StorageInstanceCharmNameSetArg{
			CharmMetadataName: "custom-charm-name",
			UUID:              storageInstUUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: internal.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: 0,
			CharmUUID:          unitCharmUUID,
			MachineUUID:        &unitMachineUUID,
		},
	}

	err := u.state.AttachStorageToUnit(c.Context(), unitUUID, attachArgs)
	c.Assert(err, tc.ErrorIsNil)

	charmName := u.getStorageInstanceCharmName(c, storageInstUUID)
	c.Check(charmName, tc.Equals, "custom-charm-name")
}

// TestAttachStorageToUnitWithExistingAttachments verifies that attaching a
// storage instance with existing attachments and a unit with existing storage
// attachments succeeds.
func (u *unitStorageSuite) TestAttachStorageToUnitWithExistingAttachments(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 2, storage,
	)
	unitUUID1 := unitUUIDs[0]
	unitUUID2 := unitUUIDs[1]

	unit1CharmUUID := u.getUnitCharmUUID(c, unitUUID1)
	unit1MachineUUID := u.getUnitMachineUUID(c, unitUUID1)
	unit1MachineNetNodeUUID := u.getMachineNetNodeUUID(c, unit1MachineUUID)

	storageInst1UUID := u.newStorageInstanceWithName(c, "st1")
	storageInst2UUID := u.newStorageInstanceWithName(c, "st1")
	u.newStorageInstanceAttachment(c, storageInst1UUID, unitUUID1)
	u.newStorageInstanceAttachment(c, storageInst2UUID, unitUUID1)

	storageInst3UUID, filesystem3UUID := u.newModelFilesystemStorageInstance(
		c, "st1", unit1CharmUUID,
	)
	existingAttachUUID := u.newStorageInstanceAttachment(
		c, storageInst3UUID, unitUUID2,
	)

	storageInstAttachUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	filesystemAttachUUID := tc.Must(c, domainstorage.NewFilesystemAttachmentUUID)

	attachArgs := internal.AttachStorageInstanceToUnitArg{
		CreateStorageInstanceAttachmentArg: internal.CreateStorageInstanceAttachmentArg{
			FilesystemAttachment: &internal.CreateUnitStorageFilesystemAttachmentArg{
				FilesystemUUID: filesystem3UUID,
				NetNodeUUID:    unit1MachineNetNodeUUID,
				ProvisionScope: domainstorageprov.ProvisionScopeMachine,
				UUID:           filesystemAttachUUID,
			},
			StorageInstanceUUID: storageInst3UUID,
			UUID:                storageInstAttachUUID,
		},
		StorageInstanceAttachmentCheckArgs: internal.StorageInstanceAttachmentCheckArgs{
			ExpectedAttachments: []domainstorage.StorageAttachmentUUID{
				existingAttachUUID,
			},
			UUID: storageInst3UUID,
		},
		UnitStorageInstanceAttachmentCheckArgs: internal.UnitStorageInstanceAttachmentCheckArgs{
			CountLessThanEqual: 2,
			CharmUUID:          unit1CharmUUID,
			MachineUUID:        &unit1MachineUUID,
		},
	}

	err := u.state.AttachStorageToUnit(c.Context(), unitUUID1, attachArgs)
	c.Assert(err, tc.ErrorIsNil)

	u.assertStorageInstanceAttachmentExists(
		c,
		storageInstAttachUUID,
		storageInst3UUID,
		unitUUID1,
	)
	u.assertFilesystemAttachmentExists(c, filesystemAttachUUID)
}

func (u *unitStorageSuite) TestGetStorageInstanceCompositionByUUIDNotFound(c *tc.C) {
	uuid := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	_, err := u.state.GetStorageInstanceCompositionByUUID(c.Context(), uuid)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
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

// TestGetStorageAttachInfoByUnitUUIDAndStorageUUIDNotFound verifies that
// looking up attach info for a missing storage instance returns an error
// satisfying [domainstorageerrors.StorageInstanceNotFound].
func (u *unitStorageSuite) TestGetStorageAttachInfoByUnitUUIDAndStorageUUIDNotFound(c *tc.C) {
	unitUUID, _ := u.newUnitWithStorageDirectives(c)
	stUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	_, err := u.state.GetStorageAttachInfoByUnitUUIDAndStorageUUID(c.Context(), unitUUID, stUUID)
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StorageInstanceNotFound)
}

// TestGetStorageAttachInfoByUnitUUIDAndStorageUUIDUnitNotFound verifies that
// looking up attach info for a missing unit returns not found.
func (u *unitStorageSuite) TestGetStorageAttachInfoByUnitUUIDAndStorageUUIDUnitNotFound(c *tc.C) {
	storageInstanceUUID, _ := u.newAliveStorageInstanceWithModelFilesystem(c)
	unitUUID := tc.Must(c, coreunit.NewUUID)

	_, err := u.state.GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		c.Context(), unitUUID, storageInstanceUUID,
	)
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetStorageAttachInfoByUnitUUIDAndStorageUUIDNotSupported verifies that
// a storage instance using a name not defined by the unit's charm is rejected.
// The caller MUST receive an error satisfying
// [applicationerrors.StorageNameNotSupported].
func (u *unitStorageSuite) TestGetStorageAttachInfoByUnitUUIDAndStorageUUIDNotSupported(c *tc.C) {
	storage := map[string]charm.Storage{
		"str1": {
			CountMax:    1,
			CountMin:    1,
			Description: "str1",
			Name:        "str1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(c, "foo", life.Alive, 1, storage)
	unitUUID := unitUUIDs[0]

	storageInstanceUUID := u.newStorageInstanceWithName(c, "str2")

	_, err := u.state.GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		c.Context(), unitUUID, storageInstanceUUID,
	)
	c.Assert(err, tc.ErrorIs, applicationerrors.StorageNameNotSupported)
}

// TestGetStorageAttachInfoByUnitUUIDAndStorageUUID verifies the happy path for
// fetching storage attach info for a unit and storage instance.
func (u *unitStorageSuite) TestGetStorageAttachInfoByUnitUUIDAndStorageUUID(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(c, "foo", life.Alive, 1, storage)
	unitUUID := unitUUIDs[0]
	unitName := u.getUnitName(c, unitUUID)
	unitMachineUUID := u.getUnitMachineUUID(c, unitUUID)
	unitNetNodeUUID := u.getUnitNetNodeUUID(c, unitUUID)

	charmUUID := u.getUnitCharmUUID(c, unitUUID)
	charmName := u.getCharmMetadataName(c, charmUUID)

	storageInstanceUUID, filesystemUUID := u.newModelFilesystemStorageInstance(
		c, "st1", charmUUID,
	)

	storageInfo, err := u.state.GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		c.Context(), unitUUID, storageInstanceUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	expected := internal.StorageInstanceInfoForUnitAttach{
		StorageInstanceInfo: internal.StorageInstanceInfo{
			UUID:             storageInstanceUUID,
			CharmName:        &charmName,
			Filesystem:       &internal.StorageInstanceFilesystemInfo{UUID: filesystemUUID, Size: 1024},
			Kind:             domainstorage.StorageKindFilesystem,
			Life:             life.Alive,
			RequestedSizeMIB: 1024,
			StorageName:      "st1",
		},
		UnitNamedStorageInfo: internal.UnitNamedStorageInfo{
			UUID:                 unitUUID,
			CharmMetadataName:    charmName,
			CharmUUID:            charmUUID,
			Name:                 coreunit.Name(unitName),
			NetNodeUUID:          unitNetNodeUUID,
			MachineUUID:          &unitMachineUUID,
			AlreadyAttachedCount: 0,
			CharmStorageDefinitionForValidation: internal.CharmStorageDefinitionForValidation{
				Name:        "st1",
				CountMin:    1,
				CountMax:    10,
				Type:        charm.StorageFilesystem,
				MinimumSize: 1024,
			},
		},
	}

	c.Assert(storageInfo, tc.DeepEquals, expected)
}

// TestGetStorageAttachInfoByUnitUUIDAndStorageUUIDAlreadyAttachedCount verifies
// that the returned count reflects existing attachments for the unit and
// storage name.
func (u *unitStorageSuite) TestGetStorageAttachInfoByUnitUUIDAndStorageUUIDAlreadyAttachedCount(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(
		c, "foo", life.Alive, 1, storage)
	unitUUID := unitUUIDs[0]
	charmUUID := u.getUnitCharmUUID(c, unitUUID)

	st1UUID, _ := u.newModelFilesystemStorageInstance(c, "st1", charmUUID)
	st2UUID, _ := u.newModelFilesystemStorageInstance(c, "st1", charmUUID)
	u.newStorageInstanceAttachment(c, st1UUID, unitUUID)
	u.newStorageInstanceAttachment(c, st2UUID, unitUUID)

	attachInfo, err := u.state.GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		c.Context(), unitUUID, st1UUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(attachInfo.UnitNamedStorageInfo.AlreadyAttachedCount, tc.Equals, uint32(2))
}

// TestGetStorageAttachInfoByUnitUUIDAndStorageUUIDAttachments verifies that
// existing storage instance attachments are returned.
func (u *unitStorageSuite) TestGetStorageAttachInfoByUnitUUIDAndStorageUUIDAttachments(c *tc.C) {
	storage := map[string]charm.Storage{
		"st1": {
			CountMax:    10,
			CountMin:    1,
			Description: "st1",
			Name:        "st1",
			MinimumSize: 1024,
			Type:        charm.StorageFilesystem,
		},
	}
	_, unitUUIDs := u.createIAASApplicationWithNUnitsAndStorage(c, "foo", life.Alive, 2, storage)
	unitUUID := unitUUIDs[0]
	otherUnitUUID := unitUUIDs[1]
	charmUUID := u.getUnitCharmUUID(c, unitUUID)

	storageInstanceUUID, _ := u.newModelFilesystemStorageInstance(c, "st1", charmUUID)
	attachUUID := u.newStorageInstanceAttachment(c, storageInstanceUUID, unitUUID)
	otherAttachUUID := u.newStorageInstanceAttachment(c, storageInstanceUUID, otherUnitUUID)

	attachInfo, err := u.state.GetStorageAttachInfoByUnitUUIDAndStorageUUID(
		c.Context(), unitUUID, storageInstanceUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(attachInfo.StorageInstanceAttachments, tc.SameContents, []internal.StorageInstanceUnitAttachment{
		{
			UnitUUID: unitUUID,
			UUID:     attachUUID,
		},
		{
			UnitUUID: otherUnitUUID,
			UUID:     otherAttachUUID,
		},
	})
}
