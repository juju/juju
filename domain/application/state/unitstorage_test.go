package state

import (
	"testing"

	"github.com/juju/clock"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	domainnetwork "github.com/juju/juju/domain/network"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/tc"
)

// unitStorageSuite is a test suite for asserting state based storage related to
// units.
type unitStorageSuite struct {
	baseSuite
}

// TestUnitStorageSuite registers and runs all of the tests located in the
// [unitStorageSuite].
func TestUnitStorageSuite(t *testing.T) {
	tc.Run(t, &unitStorageSuite{})
}

// newStorageUnitOwner is a helper function to create a new storage unit owner
// for the supplied instance and unit.
func (u *unitStorageSuite) newStorageUnitOwner(
	c *tc.C, instUUID domainstorage.StorageInstanceUUID, unitUUID coreunit.UUID,
) {
	_, err := u.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_unit_owner (storage_instance_uuid, unit_uuid) VALUES (?, ?)
`,
		instUUID.String(),
		unitUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
}

// newStorageInstanceWithModelFilesystem is a helper function to create a new
// storage instance in the model with an associated model provisioned
// filesystem.
func (u *unitStorageSuite) newStorageInstanceWithModelFilesystem(
	c *tc.C,
) (domainstorage.StorageInstanceUUID, domainstorageprov.FilesystemUUID) {
	storagePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	filesystemUUID := tc.Must(c, domainstorageprov.NewFilesystemUUID)

	_, err := u.DB().ExecContext(
		c.Context(),
		"INSERT INTO storage_pool (uuid, name, type) VALUES (?, ?, 'test')",
		storagePoolUUID.String(),
		storagePoolUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = u.DB().ExecContext(
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

// newUnit is a helper function for generating a new unit in the model for
// testing. This should be used when the caller just needs a unit with a uuid to
// exist but cares no more about the details of the unit.
func (u *unitStorageSuite) newUnit(c *tc.C) coreunit.UUID {
	st := NewState(
		u.TxnRunnerFactory(),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	appUUID, _, err := st.CreateIAASApplication(
		c.Context(),
		"foo",
		application.AddIAASApplicationArg{
			BaseAddApplicationArg: application.BaseAddApplicationArg{
				Charm: charm.Charm{
					Metadata: charm.Metadata{
						Name: "foo",
					},
					Manifest: charm.Manifest{
						Bases: []charm.Base{{
							Name:          "ubuntu",
							Channel:       charm.Channel{Risk: charm.RiskStable},
							Architectures: []string{"amd64"},
						}},
					},
					ReferenceName: "foo",
					Architecture:  architecture.AMD64,
					Revision:      1,
					Source:        charm.LocalSource,
				},
				IsController: false,
			},
		},
		[]application.AddIAASUnitArg{
			{
				AddUnitArg: application.AddUnitArg{
					NetNodeUUID: netNodeUUID,
				},
				MachineNetNodeUUID: netNodeUUID,
				MachineUUID:        tc.Must(c, coremachine.NewUUID),
				Nonce:              ptr("foo"),
			},
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	units, err := st.getApplicationUnits(c.Context(), appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(units, tc.HasLen, 1)

	return units[0]
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
