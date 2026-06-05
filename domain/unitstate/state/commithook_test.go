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
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment/charm"
	domainnetwork "github.com/juju/juju/domain/network"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/domain/unitstate/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
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
		gotState = map[string]string{}

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
	unitUUID := s.addUnitAndNetNode(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
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
	unitUUID := s.addUnitAndNetNode(c, unitName, s.fakeApplicationUUID1, s.fakeCharmUUID1)
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
	appUUID := s.addApplicationWithName(c, charmUUID, "testname", network.AlphaSpaceId.String())
	unitName := coreunit.Name("testname/0")
	unitUUID := s.addUnitAndNetNode(c, unitName, appUUID, charmUUID)

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

func (s *commitHookSuite) TestDeleteSecrets(c *tc.C) {
	ctx := c.Context()

	// Arrange: create a secret URI.
	uri := coresecrets.NewURI()

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretDeletes: []internal.DeleteSecretArg{{
			URI: uri.String(),
		}},
	}

	// Act
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	// Verify a removal job was created.
	var (
		removalTypeID int
		entityUUID    string
	)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT removal_type_id, entity_uuid FROM removal WHERE entity_uuid = ?",
			uri.String(),
		).Scan(&removalTypeID, &entityUUID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removalTypeID, tc.Equals, int(removal.CharmSecretJob))
	c.Check(entityUUID, tc.Equals, uri.String())
}

func (s *commitHookSuite) TestDeleteSecretsWithRevisions(c *tc.C) {
	ctx := c.Context()

	// Arrange: create a secret URI with specific revisions (pre-marshaled).
	uri := coresecrets.NewURI()
	revisionsJSON := `{"revisions":[1,3,5]}`

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretDeletes: []internal.DeleteSecretArg{{
			URI:     uri.String(),
			ArgJSON: &revisionsJSON,
		}},
	}

	// Act
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	// Verify a removal job was created with the revisions arg.
	var (
		removalTypeID int
		entityUUID    string
		argStr        sql.NullString
	)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT removal_type_id, entity_uuid, arg FROM removal WHERE entity_uuid = ?",
			uri.String(),
		).Scan(&removalTypeID, &entityUUID, &argStr)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(removalTypeID, tc.Equals, int(removal.CharmSecretJob))
	c.Check(entityUUID, tc.Equals, uri.String())
	c.Assert(argStr.Valid, tc.IsTrue)
	c.Check(argStr.String, tc.Equals, `{"revisions":[1,3,5]}`)
}

func (s *commitHookSuite) TestDeleteSecretsMultiple(c *tc.C) {
	ctx := c.Context()

	// Arrange: create three secret URIs with varying revision args.
	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	uri3 := coresecrets.NewURI()
	revisions24 := `{"revisions":[2,4]}`
	revisions1 := `{"revisions":[1]}`

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretDeletes: []internal.DeleteSecretArg{
			{URI: uri1.String()},
			{URI: uri2.String(), ArgJSON: &revisions24},
			{URI: uri3.String(), ArgJSON: &revisions1},
		},
	}

	// Act
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	// Verify all three removal jobs were created via bulk insert.
	type row struct {
		EntityUUID string
		Arg        sql.NullString
	}
	rows := make(map[string]row)
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows = map[string]row{}

		r, err := tx.QueryContext(ctx,
			"SELECT entity_uuid, arg FROM removal WHERE removal_type_id = ? ORDER BY entity_uuid",
			int(removal.CharmSecretJob),
		)
		if err != nil {
			return err
		}
		defer r.Close()
		for r.Next() {
			var rec row
			if err := r.Scan(&rec.EntityUUID, &rec.Arg); err != nil {
				return err
			}
			rows[rec.EntityUUID] = rec
		}
		return r.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rows, tc.HasLen, 3)

	// uri1: no revisions arg.
	c.Check(rows[uri1.String()].Arg.Valid, tc.IsFalse)

	// uri2: revisions [2, 4].
	c.Assert(rows[uri2.String()].Arg.Valid, tc.IsTrue)
	c.Check(rows[uri2.String()].Arg.String, tc.Equals, `{"revisions":[2,4]}`)

	// uri3: revisions [1].
	c.Assert(rows[uri3.String()].Arg.Valid, tc.IsTrue)
	c.Check(rows[uri3.String()].Arg.String, tc.Equals, `{"revisions":[1]}`)
}

func (s *commitHookSuite) TestRevokeSecretsAccess(c *tc.C) {
	ctx := c.Context()

	// Arrange: create a secret and a permission row.
	secretID := "secret-id-revoke-test"
	subjectUUID := s.fakeApplicationUUID1

	s.addSecret(c, secretID)
	s.addSecretPermission(c, secretID, subjectUUID, 1 /* SubjectApplication */, "some-scope-uuid", 3 /* ScopeRelation */, 0 /* RoleView */)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretRevokes: []internal.RevokeSecretArg{{
			SecretID:      secretID,
			SubjectUUID:   subjectUUID,
			SubjectTypeID: 1, // SubjectApplication
		}},
	}

	// Act
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	// Verify the permission row was deleted.
	c.Check(
		s.countRows(c,
			"SELECT count(*) FROM secret_permission WHERE secret_id = ? AND subject_uuid = ?",
			secretID, subjectUUID),
		tc.Equals, 0,
	)
}

func (s *commitHookSuite) TestRevokeSecretsAccessMultiple(c *tc.C) {
	ctx := c.Context()

	// Arrange: create two secrets with permissions.
	secretID1 := "secret-id-revoke-multi-1"
	secretID2 := "secret-id-revoke-multi-2"
	subjectUUID1 := s.fakeApplicationUUID1
	subjectUUID2 := s.fakeApplicationUUID2

	s.addSecret(c, secretID1)
	s.addSecret(c, secretID2)
	s.addSecretPermission(c, secretID1, subjectUUID1, 1, "scope-uuid-1", 3, 0)
	s.addSecretPermission(c, secretID2, subjectUUID2, 1, "scope-uuid-2", 3, 0)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretRevokes: []internal.RevokeSecretArg{
			{SecretID: secretID1, SubjectUUID: subjectUUID1, SubjectTypeID: 1},
			{SecretID: secretID2, SubjectUUID: subjectUUID2, SubjectTypeID: 1},
		},
	}

	// Act
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		s.countRows(c,
			"SELECT count(*) FROM secret_permission WHERE secret_id IN (?, ?)",
			secretID1, secretID2),
		tc.Equals, 0,
	)
}

func (s *commitHookSuite) TestRevokeSecretsAccessSecretNotFoundIsSkipped(c *tc.C) {
	ctx := c.Context()

	// Arrange: use a non-existent secret ID.
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretRevokes: []internal.RevokeSecretArg{{
			SecretID:      "nonexistent-secret-id",
			SubjectUUID:   s.fakeApplicationUUID1,
			SubjectTypeID: 1,
		}},
	}

	// Act — should not error; the missing secret is logged and skipped.
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestRevokeSecretsAccessNoPermissionIsIdempotent(c *tc.C) {
	ctx := c.Context()

	// Arrange: create a secret but no permission row for the subject.
	secretID := "secret-id-revoke-no-perm"
	s.addSecret(c, secretID)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretRevokes: []internal.RevokeSecretArg{{
			SecretID:      secretID,
			SubjectUUID:   s.fakeApplicationUUID1,
			SubjectTypeID: 1,
		}},
	}

	// Act - should not error even though no permission exists.
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *commitHookSuite) TestGrantSecretsAccess(c *tc.C) {
	ctx := c.Context()

	// Arrange: create a secret with no pre-existing permission.
	secretID := "secret-id-grant-test"
	subjectUUID := s.fakeApplicationUUID1
	scopeUUID := s.fakeApplicationUUID2

	s.addSecret(c, secretID)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretGrants: []internal.GrantSecretArg{{
			SecretID:      secretID,
			SubjectUUID:   subjectUUID,
			SubjectTypeID: 1, // SubjectApplication
			ScopeUUID:     scopeUUID,
			ScopeTypeID:   1, // ScopeApplication
			RoleID:        0, // RoleView
		}},
	}

	// Act
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		s.countRows(c,
			"SELECT count(*) FROM secret_permission WHERE secret_id = ? AND subject_uuid = ? AND role_id = 0",
			secretID, subjectUUID),
		tc.Equals, 1,
	)
}

func (s *commitHookSuite) TestGrantSecretsAccessMultiple(c *tc.C) {
	ctx := c.Context()

	// Arrange: two secrets, no pre-existing permissions.
	secretID1 := "secret-id-grant-multi-1"
	secretID2 := "secret-id-grant-multi-2"
	subjectUUID1 := s.fakeApplicationUUID1
	subjectUUID2 := s.fakeApplicationUUID2
	scopeUUID := s.fakeApplicationUUID1

	s.addSecret(c, secretID1)
	s.addSecret(c, secretID2)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretGrants: []internal.GrantSecretArg{
			{SecretID: secretID1, SubjectUUID: subjectUUID1, SubjectTypeID: 1, ScopeUUID: scopeUUID, ScopeTypeID: 1, RoleID: 0},
			{SecretID: secretID2, SubjectUUID: subjectUUID2, SubjectTypeID: 1, ScopeUUID: scopeUUID, ScopeTypeID: 1, RoleID: 0},
		},
	}

	// Act
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		s.countRows(c,
			"SELECT count(*) FROM secret_permission WHERE secret_id IN (?, ?)",
			secretID1, secretID2),
		tc.Equals, 2,
	)
}

func (s *commitHookSuite) TestGrantSecretsAccessSecretNotFoundIsSkipped(c *tc.C) {
	ctx := c.Context()

	// Arrange: use a non-existent secret ID.
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretGrants: []internal.GrantSecretArg{{
			SecretID:      "nonexistent-secret-id",
			SubjectUUID:   s.fakeApplicationUUID1,
			SubjectTypeID: 1,
			ScopeUUID:     s.fakeApplicationUUID2,
			ScopeTypeID:   1,
			RoleID:        0,
		}},
	}

	// Act — should not error; the missing secret is logged and skipped.
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		s.countRows(c, "SELECT count(*) FROM secret_permission WHERE subject_uuid = ?", s.fakeApplicationUUID1),
		tc.Equals, 0,
	)
}

func (s *commitHookSuite) TestGrantSecretsAccessInvariantViolationErrors(c *tc.C) {
	ctx := c.Context()

	// Arrange: a secret with an existing permission using scopeUUID1/scopeTypeID=1.
	// Attempting to re-grant with a different scope uuid should fail.
	secretID := "secret-id-grant-invar"
	subjectUUID := s.fakeApplicationUUID1
	scopeUUID1 := s.fakeApplicationUUID1
	scopeUUID2 := s.fakeApplicationUUID2

	s.addSecret(c, secretID)
	s.addSecretPermission(c, secretID, subjectUUID, 1, scopeUUID1, 1, 0)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretGrants: []internal.GrantSecretArg{{
			SecretID:      secretID,
			SubjectUUID:   subjectUUID,
			SubjectTypeID: 1,
			ScopeUUID:     scopeUUID2, // different scope — invariant violation
			ScopeTypeID:   1,
			RoleID:        0,
		}},
	}

	// Act
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert — invariant error must be surfaced with sentinel.
	c.Assert(err, tc.ErrorMatches, `.*cannot change scope or subject type.*`)
	c.Assert(err, tc.ErrorIs, secreterrors.InvalidSecretPermissionChange)
}

func (s *commitHookSuite) TestGrantSecretsAccessSubjectGoneIsSkipped(c *tc.C) {
	ctx := c.Context()

	// Arrange: a secret exists, but the subject UUID does not reference any
	// real entity (simulates concurrent removal between facade and commit).
	secretID := "secret-id-grant-subj-gone"
	s.addSecret(c, secretID)

	nonExistentSubjectUUID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	scopeUUID := s.fakeApplicationUUID1 // exists in application table

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretGrants: []internal.GrantSecretArg{{
			SecretID:      secretID,
			SubjectUUID:   nonExistentSubjectUUID,
			SubjectTypeID: secret.SubjectApplication,
			ScopeUUID:     scopeUUID,
			ScopeTypeID:   secret.ScopeApplication,
			RoleID:        0,
		}},
	}

	// Act — should succeed; the missing subject is logged and skipped.
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		s.countRows(c, "SELECT count(*) FROM secret_permission WHERE secret_id = ?", secretID),
		tc.Equals, 0,
	)
}

func (s *commitHookSuite) TestGrantSecretsAccessScopeGoneIsSkipped(c *tc.C) {
	ctx := c.Context()

	// Arrange: a secret exists and the subject exists, but the scope UUID
	// references a non-existent entity.
	secretID := "secret-id-grant-scope-gone"
	s.addSecret(c, secretID)

	subjectUUID := s.fakeApplicationUUID1 // exists
	nonExistentScopeUUID := "ffffffff-1111-2222-3333-444444444444"

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretGrants: []internal.GrantSecretArg{{
			SecretID:      secretID,
			SubjectUUID:   subjectUUID,
			SubjectTypeID: secret.SubjectApplication,
			ScopeUUID:     nonExistentScopeUUID,
			ScopeTypeID:   secret.ScopeApplication,
			RoleID:        0,
		}},
	}

	// Act — should succeed; the missing scope is logged and skipped.
	err := s.state.CommitHookChanges(ctx, arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(
		s.countRows(c, "SELECT count(*) FROM secret_permission WHERE secret_id = ?", secretID),
		tc.Equals, 0,
	)
}

func (s *commitHookSuite) addSecret(c *tc.C, secretID string) {
	s.query(c, `INSERT INTO secret (id) VALUES (?)`, secretID)
	s.query(c, `INSERT INTO secret_metadata (secret_id, version, rotate_policy_id) VALUES (?, 1, 0)`,
		secretID)
}

func (s *commitHookSuite) addSecretPermission(
	c *tc.C, secretID, subjectUUID string, subjectTypeID int,
	scopeUUID string, scopeTypeID, roleID int,
) {
	s.query(c, `
INSERT INTO secret_permission (secret_id, subject_uuid, subject_type_id, scope_uuid, scope_type_id, role_id)
VALUES (?, ?, ?, ?, ?, ?)`,
		secretID, subjectUUID, subjectTypeID, scopeUUID, scopeTypeID, roleID)
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

// addSecretRevision inserts a secret_revision row for the given secret and
// returns the revision UUID used.
func (s *commitHookSuite) addSecretRevision(c *tc.C, secretID string, revision int) string {
	revUUID := tc.Must(c, uuid.NewUUID).String()
	s.query(c, `
INSERT INTO secret_revision (uuid, secret_id, revision, create_time)
VALUES (?, ?, ?, DATETIME('now'))
`, revUUID, secretID, revision)
	return revUUID
}

// secretConsumerRow holds the fields we care about from secret_unit_consumer.
type secretConsumerRow struct {
	CurrentRevision int
	SourceModelUUID string
}

// getSecretConsumer reads the secret_unit_consumer row for the given secret and
// unit, returning the current_revision and source_model_uuid.
func (s *commitHookSuite) getSecretConsumer(c *tc.C, secretID, unitUUID string) (secretConsumerRow, bool) {
	var row secretConsumerRow
	var found bool
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT current_revision, source_model_uuid
FROM   secret_unit_consumer
WHERE  secret_id = ? AND unit_uuid = ?
`, secretID, unitUUID).Scan(&row.CurrentRevision, &row.SourceModelUUID)
	})
	if err == nil {
		found = true
	} else if errors.Is(err, sql.ErrNoRows) {
		found = false
		err = nil
	}
	c.Assert(err, tc.ErrorIsNil)
	return row, found
}

// TestTrackSecretsEmpty verifies that trackSecrets is a no-op when the list
// of secret IDs is empty.
func (s *commitHookSuite) TestTrackSecretsEmpty(c *tc.C) {
	err := s.state.CommitHookChanges(c.Context(), internal.CommitHookChangesArg{
		UnitUUID:           s.unitUUID,
		TrackLatestSecrets: nil,
	})
	c.Assert(err, tc.ErrorIsNil)
}

// TestTrackSecretsUnknownID verifies that a secret ID that does not exist in
// the database is silently skipped — trackSecrets must never fail for
// non-existent secrets.
func (s *commitHookSuite) TestTrackSecretsUnknownID(c *tc.C) {
	err := s.state.CommitHookChanges(c.Context(), internal.CommitHookChangesArg{
		UnitUUID:           s.unitUUID,
		TrackLatestSecrets: []string{"does-not-exist"},
	})
	c.Assert(err, tc.ErrorIsNil)

	// No consumer row must have been created.
	c.Check(
		s.countRows(c, "SELECT count(*) FROM secret_unit_consumer WHERE unit_uuid = ?", s.unitUUID),
		tc.Equals, 0,
	)
}

// TestTrackSecretsNoRevisions verifies that a secret with no revisions is
// silently skipped.
func (s *commitHookSuite) TestTrackSecretsNoRevisions(c *tc.C) {
	secretID := "tracktest-norev"
	s.addSecret(c, secretID)

	err := s.state.CommitHookChanges(c.Context(), internal.CommitHookChangesArg{
		UnitUUID:           s.unitUUID,
		TrackLatestSecrets: []string{secretID},
	})
	c.Assert(err, tc.ErrorIsNil)

	// No consumer row because there are no revisions.
	c.Check(
		s.countRows(c, "SELECT count(*) FROM secret_unit_consumer WHERE secret_id = ?", secretID),
		tc.Equals, 0,
	)
}

// TestTrackSecretsCreatesConsumerRow verifies that a secret with revisions
// gets a secret_unit_consumer row pointing to the latest revision.
func (s *commitHookSuite) TestTrackSecretsCreatesConsumerRow(c *tc.C) {
	secretID := "tracktest-create"
	s.addSecret(c, secretID)
	s.addSecretRevision(c, secretID, 1)
	s.addSecretRevision(c, secretID, 2)

	err := s.state.CommitHookChanges(c.Context(), internal.CommitHookChangesArg{
		UnitUUID:           s.unitUUID,
		TrackLatestSecrets: []string{secretID},
	})
	c.Assert(err, tc.ErrorIsNil)

	row, found := s.getSecretConsumer(c, secretID, s.unitUUID)
	c.Assert(found, tc.IsTrue)
	// Must track the latest (highest) revision.
	c.Check(row.CurrentRevision, tc.Equals, 2)
	// Local secrets have no source model UUID.
	c.Check(row.SourceModelUUID, tc.Equals, "")
}

// TestTrackSecretsUpdatesConsumerRow verifies that calling trackSecrets when a
// consumer row already exists updates current_revision to the latest value.
func (s *commitHookSuite) TestTrackSecretsUpdatesConsumerRow(c *tc.C) {
	secretID := "tracktest-update"
	s.addSecret(c, secretID)
	s.addSecretRevision(c, secretID, 1)

	// Pre-insert a consumer row that tracks revision 1.
	s.query(c, `
INSERT INTO secret_unit_consumer (secret_id, source_model_uuid, unit_uuid, label, current_revision)
VALUES (?, '', ?, 'my-label', 1)
`, secretID, s.unitUUID)

	// Add a new revision.
	s.addSecretRevision(c, secretID, 2)

	err := s.state.CommitHookChanges(c.Context(), internal.CommitHookChangesArg{
		UnitUUID:           s.unitUUID,
		TrackLatestSecrets: []string{secretID},
	})
	c.Assert(err, tc.ErrorIsNil)

	row, found := s.getSecretConsumer(c, secretID, s.unitUUID)
	c.Assert(found, tc.IsTrue)
	// Revision must now be 2 (the latest).
	c.Check(row.CurrentRevision, tc.Equals, 2)
}

// TestTrackSecretsPreservesLabel verifies that updating an existing consumer
// row does NOT overwrite the label.
func (s *commitHookSuite) TestTrackSecretsPreservesLabel(c *tc.C) {
	secretID := "tracktest-label"
	s.addSecret(c, secretID)
	s.addSecretRevision(c, secretID, 1)
	s.addSecretRevision(c, secretID, 2)
	s.query(c, `
INSERT INTO secret_unit_consumer (secret_id, source_model_uuid, unit_uuid, label, current_revision)
VALUES (?, '', ?, 'keep-this-label', 1)
`, secretID, s.unitUUID)

	err := s.state.CommitHookChanges(c.Context(), internal.CommitHookChangesArg{
		UnitUUID:           s.unitUUID,
		TrackLatestSecrets: []string{secretID},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Label must be unchanged.
	var label string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT label FROM secret_unit_consumer WHERE secret_id = ? AND unit_uuid = ?",
			secretID, s.unitUUID).Scan(&label)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(label, tc.Equals, "keep-this-label")
}

// TestTrackSecretsMarksObsoleteRevisions verifies that after tracking the
// latest revision, older revisions that are no longer consumed are marked
// obsolete (pending_delete=true) in secret_revision_obsolete.
func (s *commitHookSuite) TestTrackSecretsMarksObsoleteRevisions(c *tc.C) {
	secretID := "tracktest-obsolete"
	s.addSecret(c, secretID)
	rev1UUID := s.addSecretRevision(c, secretID, 1)
	// rev2 is the latest and will remain in-use.
	_ = s.addSecretRevision(c, secretID, 2)

	// Pre-insert consumer row tracking revision 1 — it will be moved to 2.
	s.query(c, `
INSERT INTO secret_unit_consumer (secret_id, source_model_uuid, unit_uuid, label, current_revision)
VALUES (?, '', ?, '', 1)
`, secretID, s.unitUUID)

	err := s.state.CommitHookChanges(c.Context(), internal.CommitHookChangesArg{
		UnitUUID:           s.unitUUID,
		TrackLatestSecrets: []string{secretID},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Revision 1 is no longer consumed by anyone and is not the latest →
	// it must appear in secret_revision_obsolete with pending_delete=true.
	var obsolete, pendingDelete bool
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT obsolete, pending_delete FROM secret_revision_obsolete WHERE revision_uuid = ?",
			rev1UUID).Scan(&obsolete, &pendingDelete)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obsolete, tc.IsTrue)
	c.Check(pendingDelete, tc.IsTrue)

	// Revision 2 (the latest and now consumed) must NOT be obsolete.
	c.Check(
		s.countRows(c,
			"SELECT count(*) FROM secret_revision_obsolete ro "+
				"JOIN secret_revision sr ON sr.uuid = ro.revision_uuid "+
				"WHERE sr.secret_id = ? AND sr.revision = 2 AND ro.pending_delete = true",
			secretID),
		tc.Equals, 0,
	)
}

// TestTrackSecretsMultiple verifies that multiple secrets are all tracked in
// the same CommitHookChanges call.
func (s *commitHookSuite) TestTrackSecretsMultiple(c *tc.C) {
	secretID1 := "tracktest-multi-1"
	secretID2 := "tracktest-multi-2"
	s.addSecret(c, secretID1)
	s.addSecret(c, secretID2)
	s.addSecretRevision(c, secretID1, 1)
	s.addSecretRevision(c, secretID1, 3)
	s.addSecretRevision(c, secretID2, 1)

	err := s.state.CommitHookChanges(c.Context(), internal.CommitHookChangesArg{
		UnitUUID:           s.unitUUID,
		TrackLatestSecrets: []string{secretID1, secretID2},
	})
	c.Assert(err, tc.ErrorIsNil)

	row1, found1 := s.getSecretConsumer(c, secretID1, s.unitUUID)
	c.Assert(found1, tc.IsTrue)
	c.Check(row1.CurrentRevision, tc.Equals, 3)

	row2, found2 := s.getSecretConsumer(c, secretID2, s.unitUUID)
	c.Assert(found2, tc.IsTrue)
	c.Check(row2.CurrentRevision, tc.Equals, 1)
}

// TestTrackSecretsMultiUnitObsoleteRevision verifies the critical safety
// property of markSecretRevisionsObsolete: a revision that is still tracked by
// a second consumer must NOT be marked obsolete when only the first consumer
// advances to the latest revision.
//
// Scenario:
//
//	addSecret → 2 revisions
//	consumer A (s.unitUUID) at rev 1, consumer B (unitB) at rev 1
//	trackSecrets for A only  → A advances to rev 2
//	assert rev 1 is NOT in secret_revision_obsolete  (B still tracks it)
//	trackSecrets for B        → B advances to rev 2
//	assert rev 1 IS now in secret_revision_obsolete
func (s *commitHookSuite) TestTrackSecretsMultiUnitObsoleteRevision(c *tc.C) {
	secretID := "tracktest-multiunit"
	s.addSecret(c, secretID)
	rev1UUID := s.addSecretRevision(c, secretID, 1)
	_ = s.addSecretRevision(c, secretID, 2)

	// Create a second unit in the same application.
	unitBName := coreunit.Name(s.fakeApplicationName1 + "/9")
	unitBUUID := s.addUnitAndNetNode(c, unitBName, s.fakeApplicationUUID1, s.fakeCharmUUID1).String()

	// Both consumers start at revision 1.
	s.query(c, `
INSERT INTO secret_unit_consumer (secret_id, source_model_uuid, unit_uuid, label, current_revision)
VALUES (?, '', ?, '', 1)
`, secretID, s.unitUUID)
	s.query(c, `
INSERT INTO secret_unit_consumer (secret_id, source_model_uuid, unit_uuid, label, current_revision)
VALUES (?, '', ?, '', 1)
`, secretID, unitBUUID)

	// Advance consumer A to the latest revision.
	err := s.state.CommitHookChanges(c.Context(), internal.CommitHookChangesArg{
		UnitUUID:           s.unitUUID,
		TrackLatestSecrets: []string{secretID},
	})
	c.Assert(err, tc.ErrorIsNil)

	rowA, found := s.getSecretConsumer(c, secretID, s.unitUUID)
	c.Assert(found, tc.IsTrue)
	c.Check(rowA.CurrentRevision, tc.Equals, 2)

	// Revision 1 must NOT be obsolete yet — unit B still tracks it.
	c.Check(
		s.countRows(c,
			"SELECT count(*) FROM secret_revision_obsolete WHERE revision_uuid = ?",
			rev1UUID),
		tc.Equals, 0,
	)

	// Now advance consumer B to the latest revision.
	err = s.state.CommitHookChanges(c.Context(), internal.CommitHookChangesArg{
		UnitUUID:           unitBUUID,
		TrackLatestSecrets: []string{secretID},
	})
	c.Assert(err, tc.ErrorIsNil)

	rowB, found := s.getSecretConsumer(c, secretID, unitBUUID)
	c.Assert(found, tc.IsTrue)
	c.Check(rowB.CurrentRevision, tc.Equals, 2)

	// Now revision 1 is no longer tracked by anyone → it must be obsolete.
	var obsolete, pendingDelete bool
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT obsolete, pending_delete FROM secret_revision_obsolete WHERE revision_uuid = ?",
			rev1UUID).Scan(&obsolete, &pendingDelete)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obsolete, tc.IsTrue)
	c.Check(pendingDelete, tc.IsTrue)
}

// TestTrackSecretsIdempotent verifies that calling CommitHookChanges twice
// with the same TrackLatestSecrets entry produces no spurious side-effects:
//   - current_revision still reflects the latest revision
//   - no revision that is still current ends up in secret_revision_obsolete
//     with pending_delete=true
func (s *commitHookSuite) TestTrackSecretsIdempotent(c *tc.C) {
	secretID := "tracktest-idempotent"
	s.addSecret(c, secretID)
	s.addSecretRevision(c, secretID, 1)
	_ = s.addSecretRevision(c, secretID, 2)

	arg := internal.CommitHookChangesArg{
		UnitUUID:           s.unitUUID,
		TrackLatestSecrets: []string{secretID},
	}

	// First call — creates the consumer row.
	c.Assert(s.state.CommitHookChanges(c.Context(), arg), tc.ErrorIsNil)

	// Second call — upserts the same row; must be a no-op.
	c.Assert(s.state.CommitHookChanges(c.Context(), arg), tc.ErrorIsNil)

	row, found := s.getSecretConsumer(c, secretID, s.unitUUID)
	c.Assert(found, tc.IsTrue)
	// Must still point to the latest revision.
	c.Check(row.CurrentRevision, tc.Equals, 2)

	// The current (latest) revision must NOT be marked pending_delete.
	c.Check(
		s.countRows(c,
			"SELECT count(*) FROM secret_revision_obsolete ro "+
				"JOIN secret_revision sr ON sr.uuid = ro.revision_uuid "+
				"WHERE sr.secret_id = ? AND sr.revision = 2 AND ro.pending_delete = true",
			secretID),
		tc.Equals, 0,
	)
}
