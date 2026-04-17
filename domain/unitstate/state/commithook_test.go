// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment/charm"
	domainnetwork "github.com/juju/juju/domain/network"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/domain/unitstate/internal"
)

type commitHookSuite struct {
	commitHookBaseSuite
}

func TestCommitHookSuite(t *testing.T) {
	tc.Run(t, &commitHookSuite{})
}

func (s *commitHookSuite) TestCommitHookChanges(c *tc.C) {
	// Arrange
	arg := internal.CommitHookChangesArg{
		UnitUUID:           s.unitUUID,
		UpdateNetworkInfo:  true,
		RelationSettings:   nil,
		OpenPorts:          nil,
		ClosePorts:         nil,
		CharmState:         nil,
		SecretCreates:      nil,
		TrackLatestSecrets: nil,
		SecretUpdates:      nil,
		SecretGrants:       nil,
		SecretRevokes:      nil,
		SecretDeletes:      nil,
	}

	// Act
	err := s.state.CommitHookChanges(c.Context(), arg)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *commitHookSuite) TestUpdateCharmState(c *tc.C) {
	ctx := c.Context()

	// Arrange
	// Set some initial state. This should be overwritten.
	s.addUnitStateCharm(c, "one-key", "one-val")

	expState := map[string]string{
		"two-key":   "two-val",
		"three-key": "three-val",
	}

	// Act
	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unit := entityUUID{UUID: s.unitUUID}
		return s.state.updateCharmState(ctx, tx, unit, &expState)
	})
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	gotState := make(map[string]string)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT key, value FROM unit_state_charm WHERE unit_uuid = ?"
		rows, err := tx.QueryContext(ctx, q, s.unitUUID)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				return err
			}
			gotState[k] = v
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(gotState, tc.DeepEquals, expState)
}

func (s *commitHookSuite) TestUpdateCharmStateEmpty(c *tc.C) {
	ctx := c.Context()

	// Act - use a bad unit uuid to ensure the test fails if setUnitStateCharm
	// is called.
	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		unit := entityUUID{UUID: "bad-unit-uuid"}
		return s.state.updateCharmState(ctx, tx, unit, nil)
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestCommitHookRelationSettings(c *tc.C) {
	// Arrange: Add relation with one endpoint.
	endpoint1 := domainrelation.Endpoint{
		ApplicationName: s.fakeApplicationName1,
		Relation: charm.Relation{
			Name:      "fake-endpoint-name-1",
			Role:      charm.RoleProvider,
			Interface: "database",
			Scope:     charm.ScopeContainer,
		},
	}
	charmRelationUUID1 := s.addCharmRelation(c, s.fakeCharmUUID1, endpoint1.Relation)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, s.fakeApplicationUUID1, charmRelationUUID1)
	relationUUID := s.addRelation(c)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)

	// Arrange: Add a unit to the relation.
	unitName := coreunittesting.GenNewName(c, "app/7")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	relationUnitUUID := s.addRelationUnit(c, unitUUID, relationEndpointUUID1)

	// Arrange: setup the method input
	appSettings := map[string]string{
		"key2": "value2",
		"key3": "value3",
	}
	unitSettings := map[string]string{
		"key1": "value1",
		"key3": "value3",
	}
	arg := internal.CommitHookChangesArg{
		UnitUUID: unitUUID.String(),
		RelationSettings: []internal.RelationSettings{{
			RelationUUID:   relationUUID,
			ApplicationSet: appSettings,
			UnitSet:        unitSettings,
		}},
	}

	// Act
	err := s.state.CommitHookChanges(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	foundAppSettings := s.getRelationApplicationSettings(c, relationEndpointUUID1)
	c.Check(foundAppSettings, tc.DeepEquals, appSettings)
	foundUnitSettings := s.getRelationUnitSettings(c, relationUnitUUID)
	c.Check(foundUnitSettings, tc.DeepEquals, unitSettings)
}

func (s *commitHookSuite) TestCommitHookAddStorage(c *tc.C) {
	poolUUID := s.addStoragePool(c, "test-pool", "lxd")
	unitUUID := s.unitUUID
	netNodeUUID := s.getUnitNetNodeUUID(c, s.unitUUID)
	machineUUID := s.getUnitMachineUUID(c, s.unitUUID)

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	volumeUUID := tc.Must(c, domainstorage.NewVolumeUUID)
	storageAttachmentUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	volumeAttachmentUUID := tc.Must(c, domainstorage.NewVolumeAttachmentUUID)

	arg := internal.CommitHookChangesArg{
		UnitUUID:    unitUUID,
		MachineUUID: &machineUUID,
		AddStorage: []unitstate.PreparedStorageAdd{{
			StorageName: "data",
			Storage: domainstorage.IAASUnitAddStorageArg{
				UnitAddStorageArg: domainstorage.UnitAddStorageArg{
					StorageInstances: []domainstorage.CreateUnitStorageInstanceArg{{
						UUID:            storageInstanceUUID,
						CharmName:       "app",
						Kind:            domainstorage.StorageKindBlock,
						Name:            "data",
						RequestSizeMiB:  1024,
						StoragePoolUUID: poolUUID,
						Volume: &domainstorage.CreateUnitStorageVolumeArg{
							UUID:           volumeUUID,
							ProvisionScope: domainstorage.ProvisionScopeMachine,
						},
					}},
					StorageToAttach: []domainstorage.CreateUnitStorageAttachmentArg{{
						UUID:                storageAttachmentUUID,
						StorageInstanceUUID: storageInstanceUUID,
						VolumeAttachment: &domainstorage.CreateUnitStorageVolumeAttachmentArg{
							UUID:           volumeAttachmentUUID,
							NetNodeUUID:    domainnetwork.NetNodeUUID(netNodeUUID),
							ProvisionScope: domainstorage.ProvisionScopeMachine,
							VolumeUUID:     volumeUUID,
						},
					}},
					StorageToOwn:       []domainstorage.StorageInstanceUUID{storageInstanceUUID},
					CountLessThanEqual: 0,
				},
				VolumesToOwn: []domainstorage.VolumeUUID{
					volumeUUID,
				},
			},
		}},
	}

	err := s.state.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(
		s.countRows(c, "SELECT count(*) FROM storage_instance WHERE uuid = ?", storageInstanceUUID.String()),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(
			c, "SELECT count(*) FROM storage_attachment WHERE storage_instance_uuid = ? AND unit_uuid = ?",
			storageInstanceUUID.String(),
			s.unitUUID,
		),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(
			c, "SELECT count(*) FROM storage_unit_owner WHERE storage_instance_uuid = ? AND unit_uuid = ?",
			storageInstanceUUID.String(),
			s.unitUUID,
		),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(c, "SELECT count(*) FROM storage_volume WHERE uuid = ?", volumeUUID.String()),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(
			c, "SELECT count(*) FROM machine_volume WHERE machine_uuid = ? AND volume_uuid = ?",
			machineUUID,
			volumeUUID.String(),
		),
		tc.Equals,
		1,
	)
}

func (s *commitHookSuite) TestCommitHookAddStorageVolumeBackedFilesystem(c *tc.C) {
	poolUUID := s.addStoragePool(c, "test-pool", "lxd")
	unitUUID := s.unitUUID
	netNodeUUID := s.getUnitNetNodeUUID(c, s.unitUUID)
	machineUUID := s.getUnitMachineUUID(c, s.unitUUID)

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	volumeUUID := tc.Must(c, domainstorage.NewVolumeUUID)
	filesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	storageAttachmentUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	volumeAttachmentUUID := tc.Must(c, domainstorage.NewVolumeAttachmentUUID)
	fsAttachmentUUID := tc.Must(c, domainstorage.NewFilesystemAttachmentUUID)

	arg := internal.CommitHookChangesArg{
		UnitUUID:    unitUUID,
		MachineUUID: &machineUUID,
		AddStorage: []unitstate.PreparedStorageAdd{{
			StorageName: "data",
			Storage: domainstorage.IAASUnitAddStorageArg{
				UnitAddStorageArg: domainstorage.UnitAddStorageArg{
					StorageInstances: []domainstorage.CreateUnitStorageInstanceArg{{
						UUID:            storageInstanceUUID,
						CharmName:       "app",
						Kind:            domainstorage.StorageKindFilesystem,
						Name:            "data",
						RequestSizeMiB:  1024,
						StoragePoolUUID: poolUUID,
						Volume: &domainstorage.CreateUnitStorageVolumeArg{
							UUID:           volumeUUID,
							ProvisionScope: domainstorage.ProvisionScopeMachine,
						},
						Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
							UUID:           filesystemUUID,
							ProvisionScope: domainstorage.ProvisionScopeMachine,
						},
					}},
					StorageToAttach: []domainstorage.CreateUnitStorageAttachmentArg{{
						UUID:                storageAttachmentUUID,
						StorageInstanceUUID: storageInstanceUUID,
						VolumeAttachment: &domainstorage.CreateUnitStorageVolumeAttachmentArg{
							UUID:           volumeAttachmentUUID,
							NetNodeUUID:    domainnetwork.NetNodeUUID(netNodeUUID),
							ProvisionScope: domainstorage.ProvisionScopeMachine,
							VolumeUUID:     volumeUUID,
						},
						FilesystemAttachment: &domainstorage.CreateUnitStorageFilesystemAttachmentArg{
							UUID:           fsAttachmentUUID,
							NetNodeUUID:    domainnetwork.NetNodeUUID(netNodeUUID),
							ProvisionScope: domainstorage.ProvisionScopeMachine,
							FilesystemUUID: filesystemUUID,
						},
					}},
					StorageToOwn:       []domainstorage.StorageInstanceUUID{storageInstanceUUID},
					CountLessThanEqual: 0,
				},
				VolumesToOwn: []domainstorage.VolumeUUID{
					volumeUUID,
				},
				FilesystemsToOwn: []domainstorage.FilesystemUUID{
					filesystemUUID,
				},
			},
		}},
	}

	err := s.state.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(
		s.countRows(c, "SELECT count(*) FROM storage_instance WHERE uuid = ?", storageInstanceUUID.String()),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(
			c, "SELECT count(*) FROM storage_attachment WHERE storage_instance_uuid = ? AND unit_uuid = ?",
			storageInstanceUUID.String(),
			s.unitUUID,
		),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(
			c, "SELECT count(*) FROM storage_unit_owner WHERE storage_instance_uuid = ? AND unit_uuid = ?",
			storageInstanceUUID.String(),
			s.unitUUID,
		),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(c, "SELECT count(*) FROM storage_volume WHERE uuid = ?", volumeUUID.String()),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(
			c, "SELECT count(*) FROM machine_volume WHERE machine_uuid = ? AND volume_uuid = ?",
			machineUUID,
			volumeUUID.String(),
		),
		tc.Equals,
		1,
	)
}

func (s *commitHookSuite) TestCommitHookAddStorageWithoutMachineOwnership(c *tc.C) {
	poolUUID := s.addStoragePool(c, "test-pool", "lxd")
	unitName := coreunittesting.GenNewName(c, "app/8")
	unitUUID := s.addUnit(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
	netNodeUUID := s.getUnitNetNodeUUID(c, unitUUID.String())

	storageInstanceUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)
	filesystemUUID := tc.Must(c, domainstorage.NewFilesystemUUID)
	storageAttachmentUUID := tc.Must(c, domainstorage.NewStorageAttachmentUUID)
	filesystemAttachmentUUID := tc.Must(c, domainstorage.NewFilesystemAttachmentUUID)

	arg := internal.CommitHookChangesArg{
		UnitUUID: unitUUID.String(),
		AddStorage: []unitstate.PreparedStorageAdd{{
			StorageName: "data",
			Storage: domainstorage.IAASUnitAddStorageArg{
				UnitAddStorageArg: domainstorage.UnitAddStorageArg{
					StorageInstances: []domainstorage.CreateUnitStorageInstanceArg{{
						UUID:            storageInstanceUUID,
						CharmName:       "app",
						Kind:            domainstorage.StorageKindFilesystem,
						Name:            "data",
						RequestSizeMiB:  1024,
						StoragePoolUUID: poolUUID,
						Filesystem: &domainstorage.CreateUnitStorageFilesystemArg{
							UUID:           filesystemUUID,
							ProvisionScope: domainstorage.ProvisionScopeModel,
						},
					}},
					StorageToAttach: []domainstorage.CreateUnitStorageAttachmentArg{{
						UUID:                storageAttachmentUUID,
						StorageInstanceUUID: storageInstanceUUID,
						FilesystemAttachment: &domainstorage.CreateUnitStorageFilesystemAttachmentArg{
							UUID:           filesystemAttachmentUUID,
							NetNodeUUID:    domainnetwork.NetNodeUUID(netNodeUUID),
							ProvisionScope: domainstorage.ProvisionScopeModel,
							FilesystemUUID: filesystemUUID,
						},
					}},
					StorageToOwn:       []domainstorage.StorageInstanceUUID{storageInstanceUUID},
					CountLessThanEqual: 0,
				},
			},
		}},
	}

	err := s.state.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(
		s.countRows(c, "SELECT count(*) FROM storage_instance WHERE uuid = ?", storageInstanceUUID.String()),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(c, "SELECT count(*) FROM storage_filesystem WHERE uuid = ?", filesystemUUID.String()),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(
			c, "SELECT count(*) FROM storage_attachment WHERE storage_instance_uuid = ? AND unit_uuid = ?",
			storageInstanceUUID.String(),
			unitUUID.String(),
		),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(
			c,
			"SELECT count(*) FROM storage_filesystem_attachment WHERE storage_filesystem_uuid = ? AND net_node_uuid = ?",
			filesystemUUID.String(),
			netNodeUUID,
		),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(
			c, "SELECT count(*) FROM storage_unit_owner WHERE storage_instance_uuid = ? AND unit_uuid = ?",
			storageInstanceUUID.String(),
			unitUUID.String(),
		),
		tc.Equals,
		1,
	)
	c.Check(
		s.countRows(c, "SELECT count(*) FROM machine_filesystem WHERE filesystem_uuid = ?", filesystemUUID.String()),
		tc.Equals,
		0,
	)
}

func (s *commitHookSuite) TestCommitHookAddStorageCountPreconditionFailed(c *tc.C) {
	poolUUID := s.addStoragePool(c, "test-pool", "lxd")
	existingStorageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	s.query(c, `
INSERT INTO storage_instance
    (uuid, charm_name, storage_name, storage_kind_id, storage_id, life_id,
     storage_pool_uuid, requested_size_mib)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, existingStorageUUID.String(), "app", "data",
		int(domainstorage.StorageKindBlock), "data/0", 0,
		poolUUID.String(), 1024)
	s.query(c, `
INSERT INTO storage_unit_owner (storage_instance_uuid, unit_uuid)
VALUES (?, ?)
`, existingStorageUUID.String(), s.unitUUID)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		AddStorage: []unitstate.PreparedStorageAdd{{
			StorageName: "data",
			Storage: domainstorage.IAASUnitAddStorageArg{
				UnitAddStorageArg: domainstorage.UnitAddStorageArg{
					CountLessThanEqual: 0,
				},
			},
		}},
	}

	err := s.state.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIs, storageerrors.MaxStorageCountPreconditionFailed)
	c.Check(
		s.countRows(c, "SELECT count(*) FROM storage_instance WHERE storage_name = ?", "data"), tc.Equals, 1)
}

func (s *commitHookSuite) TestCommitHookAddStorageRollsBackEarlierChanges(c *tc.C) {
	poolUUID := s.addStoragePool(c, "test-pool", "lxd")
	existingStorageUUID := tc.Must(c, domainstorage.NewStorageInstanceUUID)

	s.query(c, `
INSERT INTO storage_instance
    (uuid, charm_name, storage_name, storage_kind_id, storage_id, life_id,
     storage_pool_uuid, requested_size_mib)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, existingStorageUUID.String(), "app", "data",
		int(domainstorage.StorageKindBlock), "data/0", 0,
		poolUUID.String(), 1024)
	s.query(c, `
INSERT INTO storage_unit_owner (storage_instance_uuid, unit_uuid)
VALUES (?, ?)
`, existingStorageUUID.String(), s.unitUUID)

	charmState := map[string]string{"foo": "bar"}
	arg := internal.CommitHookChangesArg{
		UnitUUID:   s.unitUUID,
		CharmState: &charmState,
		AddStorage: []unitstate.PreparedStorageAdd{{
			StorageName: "data",
			Storage: domainstorage.IAASUnitAddStorageArg{
				UnitAddStorageArg: domainstorage.UnitAddStorageArg{
					CountLessThanEqual: 0,
				},
			},
		}},
	}

	err := s.state.CommitHookChanges(c.Context(), arg)
	c.Assert(err, tc.ErrorIs, storageerrors.MaxStorageCountPreconditionFailed)
	c.Check(
		s.countRows(c, "SELECT count(*) FROM unit_state_charm WHERE unit_uuid = ? AND key = ?", s.unitUUID, "foo"),
		tc.Equals,
		0,
	)
	c.Check(
		s.countRows(c, "SELECT count(*) FROM storage_instance WHERE storage_name = ?", "data"),
		tc.Equals,
		1,
	)
}

func (s *commitHookSuite) TestGetCommitHookUnitInfo(c *tc.C) {
	unitName := s.unitName
	expectedUUID := s.unitUUID
	expectedMachineUUID := s.getUnitMachineUUID(c, s.unitUUID)

	// Act
	unitInfo, err := s.state.GetCommitHookUnitInfo(c.Context(), unitName)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitInfo.UnitUUID, tc.Equals, expectedUUID)
	c.Check(unitInfo.MachineUUID, tc.Deref(tc.Equals), expectedMachineUUID)
}

func (s *commitHookSuite) TestGetCommitHookUnitInfoNotFound(c *tc.C) {
	_, err := s.state.GetCommitHookUnitInfo(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *commitHookSuite) TestEnsureCheckRelationExistsNotFound(c *tc.C) {
	// Arrange: add a unit
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, "testname", network.AlphaSpaceId.String())
	unitName := coreunit.Name("testname/0")
	unitUUID := s.addUnit(c, unitName, appUUID, charmUUID)

	// Arrange: setup the method input with a non-existent relation uuid
	arg := internal.CommitHookChangesArg{
		UnitUUID: unitUUID.String(),
		RelationSettings: []internal.RelationSettings{{
			RelationUUID: tc.Must(c, corerelation.NewUUID),
		}},
	}

	// Act
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.checkRelationsExist(ctx, tx, arg.RelationSettings)
	})

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *commitHookSuite) addStoragePool(
	c *tc.C,
	name, providerType string,
) domainstorage.StoragePoolUUID {
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	s.query(c, `INSERT INTO storage_pool (uuid, name, type) VALUES (?, ?, ?)`,
		poolUUID.String(), name, providerType)
	return poolUUID
}

func (s *commitHookSuite) getUnitNetNodeUUID(c *tc.C, unitUUID string) string {
	var netNodeUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(
			ctx,
			"SELECT net_node_uuid FROM unit WHERE uuid = ?",
			unitUUID,
		).Scan(&netNodeUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	return netNodeUUID
}

func (s *commitHookSuite) getUnitMachineUUID(c *tc.C, unitUUID string) string {
	var machineUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT m.uuid
FROM unit u
JOIN machine m ON m.net_node_uuid = u.net_node_uuid
WHERE u.uuid = ?
`, unitUUID).Scan(&machineUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	return machineUUID
}

func (s *commitHookSuite) countRows(c *tc.C, query string, args ...any) int {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, query, args...).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	return count
}
