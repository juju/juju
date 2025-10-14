// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// unitStorageSuite is a test suite for asserting state based storage related to
// units.
type unitStorageSuite struct {
	baseSuite

	storageHelper
}

// TestUnitStorageSuite registers and runs all of the tests located in the
// [unitStorageSuite].
func TestUnitStorageSuite(t *testing.T) {
	suite := &unitStorageSuite{}
	suite.storageHelper.dbGetter = &suite.ModelSuite
	tc.Run(t, suite)
}

func (u *unitStorageSuite) newCharmWithStorage(
	c *tc.C,
	name string,
	storage map[string]charm.Storage,
) corecharm.ID {
	ch := charm.Charm{
		Metadata: charm.Metadata{
			Name:    name,
			Storage: storage,
		},
		Manifest:      u.minimalManifest(c),
		Config:        charm.Config{},
		ReferenceName: name,
		Source:        charm.LocalSource,
		Revision:      43,
	}

	st := NewState(
		u.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	charmID, _, err := st.AddCharm(c.Context(), ch, nil, false)
	c.Assert(err, tc.ErrorIsNil)
	return charmID
}

// newStorageInstanceWithModelFilesystem is a helper function to create a new
// storage instance in the model with an associated model provisioned
// filesystem.
func (u *unitStorageSuite) newStorageInstanceWithModelFilesystem(
	c *tc.C,
) (domainstorage.StorageInstanceUUID, domainstorageprov.FilesystemUUID) {
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	filesystemUUID := tc.Must(c, domainstorageprov.NewFilesystemUUID)

	storagePoolUUID := u.newStoragePool(c, storageInstanceUUID.String(), "test-provider")

	_, err := u.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_instance (uuid, storage_name, storage_kind_id, storage_id,
                              life_id, storage_pool_uuid, requested_size_mib)
VALUES (?, ?, 1, ?, 1, ?, 1024)
`,
		storageInstanceUUID.String(),
		storageInstanceUUID.String(),
		storageInstanceUUID.String(),
		storagePoolUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = u.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 1, 0)
	`,
		filesystemUUID.String(),
		filesystemUUID.String(),
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

	st := NewState(
		u.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	_, err := st.GetUnitOwnedStorageInstances(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetUnitOwnedStorageInstancesNotStorage tests that if the unit has no
// storage that it owns no error is returned and an empty results set is
// provided.
func (u *unitStorageSuite) TestGetUnitOwnedStorageInstancesNotStorage(c *tc.C) {
	unitUUID := u.newUnit(c)

	st := NewState(
		u.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	insts, err := st.GetUnitOwnedStorageInstances(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(insts, tc.HasLen, 0)
}

func (u *unitStorageSuite) TestGetUnitOwnedStorageInstances(c *tc.C) {
	unitUUID := u.newUnit(c)
	st1UUID, fs1UUID := u.newStorageInstanceWithModelFilesystem(c)
	st2UUID, fs2UUID := u.newStorageInstanceWithModelFilesystem(c)
	u.newStorageUnitOwner(c, st1UUID, unitUUID)
	u.newStorageUnitOwner(c, st2UUID, unitUUID)

	st := NewState(
		u.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	owned, err := st.GetUnitOwnedStorageInstances(c.Context(), unitUUID)
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

// TestGetUnitStorageDirectives tests the happy path of getting a units storage
// directives.
func (u *unitStorageSuite) TestGetUnitStorageDirectives(c *tc.C) {
	charmUUID := u.newCharmWithStorage(
		c,
		"test-charm",
		map[string]charm.Storage{
			"st1": charm.Storage{
				CountMax:    10,
				CountMin:    1,
				Description: "st1",
				Name:        "st1",
				MinimumSize: 1024,
				Type:        charm.StorageFilesystem,
			},
			"st2": charm.Storage{
				CountMax:    1,
				CountMin:    1,
				Description: "st2",
				Name:        "st2",
				MinimumSize: 2048,
				Type:        charm.StorageBlock,
			},
		})
	unitUUID := u.newUnit(c)
	storagePoolUUID := u.newStoragePool(c, "test-pool", "test-provider")

	_, err := u.DB().ExecContext(
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

	st := NewState(
		u.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	gotDirectives, err := st.GetUnitStorageDirectives(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(gotDirectives, tc.SameContents, []application.StorageDirective{
		{
			CharmMetadataName: "test-charm",
			CharmStorageType:  charm.StorageBlock,
			Count:             1,
			MaxCount:          1,
			Name:              domainstorage.Name("st2"),
			PoolUUID:          storagePoolUUID,
			Size:              8000,
		},
		{
			CharmMetadataName: "test-charm",
			CharmStorageType:  charm.StorageFilesystem,
			Count:             4,
			MaxCount:          10,
			Name:              domainstorage.Name("st1"),
			PoolUUID:          storagePoolUUID,
			Size:              5000,
		},
	})
}

// TestGetUnitStorageDirectivesEmpty ensures that when a unit has no storage
func (u *unitStorageSuite) TestGetUnitStorageDirectivesEmpty(c *tc.C) {
	unitUUID := u.newUnit(c)

	st := NewState(
		u.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	directives, err := st.GetUnitStorageDirectives(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(directives, tc.HasLen, 0)
}

// TestGetUnitStorageDirectivesUnitNotFound ensures that when asking for the
// storage directives of a unit that does not exist in the model the caller gets
// back a [applicationerrors.UnitNotFound] error.
func (u *unitStorageSuite) TestGetUnitStorageDirectivesUnitNotFound(c *tc.C) {
	unitUUID := tc.Must(c, coreunit.NewUUID)
	st := NewState(
		u.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	_, err := st.GetUnitStorageDirectives(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}
