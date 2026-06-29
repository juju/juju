// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"
	"time"

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
	s.query(c, `INSERT INTO secret_metadata (secret_id, version, rotate_policy_id, create_time, update_time) VALUES (?, 1, 0, DATETIME('now', 'utc'), DATETIME('now', 'utc'))`,
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
	// Local secrets store the model UUID as source_model_uuid, matching the
	// invariant set by SaveSecretConsumer.
	c.Check(row.SourceModelUUID, tc.Equals, s.modelUUID.String())
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

// TestUpdateSecretsMetadata verifies that updating secret metadata (without new
// content) correctly updates description, rotate policy, and checksum.
func (s *commitHookSuite) TestUpdateSecretsMetadata(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-metadata"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	s.addSecretRevision(c, secretID, 1)

	policy := secret.RotateDaily
	desc := "updated description"
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			RevisionUUID: "new-uuid",
			SecretID:     secretID,
			RotatePolicy: &policy,
			Description:  &desc,
			Checksum:     "new-checksum",
			OwnerKind:    secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var gotDesc string
	var gotPolicyID int
	var gotChecksum string
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT description, rotate_policy_id, latest_revision_checksum FROM secret_metadata WHERE secret_id = ?",
			secretID).Scan(&gotDesc, &gotPolicyID, &gotChecksum)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotDesc, tc.Equals, desc)
	c.Check(gotPolicyID, tc.Equals, int(policy))
	c.Check(gotChecksum, tc.Equals, "new-checksum")
}

// TestUpdateSecretsWithNewData verifies that updating with new data creates a
// new revision with the correct content.
func (s *commitHookSuite) TestUpdateSecretsWithNewData(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-data"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	s.addSecretRevision(c, secretID, 1)

	revUUID := tc.Must(c, uuid.NewUUID).String()
	data := map[string]string{"key1": "value1", "key2": "value2"}
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:     secretID,
			Data:         data,
			Checksum:     "checksum-with-data",
			RevisionUUID: revUUID,
			OwnerKind:    secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	// Verify revision was created
	var revCount int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_revision WHERE secret_id = ?",
			secretID).Scan(&revCount)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(revCount, tc.Equals, 2)

	// Verify content
	var contentCount int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_content WHERE revision_uuid = ?",
			revUUID).Scan(&contentCount)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(contentCount, tc.Equals, 2)
}

// TestUpdateSecretsApplicationOwned verifies that updating an application-owned
// secret works correctly.
func (s *commitHookSuite) TestUpdateSecretsApplicationOwned(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-app-owned"
	s.addSecretWithOwner(c, secretID, s.fakeApplicationUUID1, "application")
	s.addSecretRevision(c, secretID, 1)

	policy := secret.RotateWeekly
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:     secretID,
			RotatePolicy: &policy,
			Checksum:     "app-checksum",
			OwnerKind:    secret.ApplicationCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var gotPolicyID int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT rotate_policy_id FROM secret_metadata WHERE secret_id = ?",
			secretID).Scan(&gotPolicyID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotPolicyID, tc.Equals, int(policy))
}

// TestUpdateSecretsSecretNotFoundIsSkipped verifies that attempting to update a
// non-existent secret is skipped without error.
func (s *commitHookSuite) TestUpdateSecretsSecretNotFoundIsSkipped(c *tc.C) {
	ctx := c.Context()

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:  "nonexistent-secret",
			Checksum:  "checksum",
			OwnerKind: secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)
}

// TestUpdateSecretsWithLabel verifies that updating a secret label works for
// both unit and application owners, and preserves the existing checksum
// (since metadata-only updates should not clobber the latest content revision's checksum).
func (s *commitHookSuite) TestUpdateSecretsWithLabel(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-label"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	s.addSecretRevision(c, secretID, 1)

	// First, set a real checksum for the revision.
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"UPDATE secret_metadata SET latest_revision_checksum = ? WHERE secret_id = ?",
			"original-checksum", secretID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	newLabel := "new-label"
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:  secretID,
			Label:     &newLabel,
			Checksum:  "", // metadata-only: empty checksum from uniter
			OwnerKind: secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var gotLabel, gotChecksum string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT label FROM secret_unit_owner WHERE secret_id = ?",
			secretID).Scan(&gotLabel)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotLabel, tc.Equals, newLabel)

	// Verify the checksum is preserved, not clobbered by the empty value from the metadata-only update.
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT latest_revision_checksum FROM secret_metadata WHERE secret_id = ?",
			secretID).Scan(&gotChecksum)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotChecksum, tc.Equals, "original-checksum")

	// Verify no new revision was created for a metadata-only update.
	var revCount int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_revision WHERE secret_id = ?",
			secretID).Scan(&revCount)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(revCount, tc.Equals, 1)
}

// TestUpdateSecretsMetadataOnlyPreservesChecksum verifies that updating a secret
// with only metadata changes (description, label, rotation) preserves the checksum
// of the latest content revision and does not create a new revision.
func (s *commitHookSuite) TestUpdateSecretsMetadataOnlyPreservesChecksum(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-metadata-checksum"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	s.addSecretRevision(c, secretID, 1)

	// Set a real checksum for the revision.
	originalChecksum := "original-content-checksum"
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"UPDATE secret_metadata SET latest_revision_checksum = ? WHERE secret_id = ?",
			originalChecksum, secretID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Update only the description (metadata-only).
	newDesc := "new description"
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:    secretID,
			Description: &newDesc,
			Checksum:    "", // metadata-only: uniter sends empty checksum
			OwnerKind:   secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	// Verify description was updated.
	var gotDesc, gotChecksum string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT description, latest_revision_checksum FROM secret_metadata WHERE secret_id = ?",
			secretID).Scan(&gotDesc, &gotChecksum)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotDesc, tc.Equals, newDesc)

	// Verify checksum is preserved, not clobbered.
	c.Check(gotChecksum, tc.Equals, originalChecksum)

	// Verify no new revision was created.
	var revCount int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_revision WHERE secret_id = ?",
			secretID).Scan(&revCount)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(revCount, tc.Equals, 1)
}

// TestUpdateSecretsMarksObsolete verifies that after updating a secret, the old
// revision is marked obsolete.
func (s *commitHookSuite) TestUpdateSecretsMarksObsolete(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-obsolete"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	rev1UUID := s.addSecretRevision(c, secretID, 1)

	rev2UUID := tc.Must(c, uuid.NewUUID).String()
	data := map[string]string{"key": "value"}
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:     secretID,
			Data:         data,
			Checksum:     "checksum-obsolete",
			RevisionUUID: rev2UUID,
			OwnerKind:    secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var obsolete, pendingDelete bool
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT obsolete, pending_delete FROM secret_revision_obsolete WHERE revision_uuid = ?",
			rev1UUID).Scan(&obsolete, &pendingDelete)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obsolete, tc.IsTrue)
	c.Check(pendingDelete, tc.IsTrue)
}

// TestUpdateSecretsWithValueRef verifies that updating a secret with an external
// backend ValueRef creates a revision without in-band secret_content rows.
func (s *commitHookSuite) TestUpdateSecretsWithValueRef(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-valueref"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	s.addSecretRevision(c, secretID, 1)

	revUUID := tc.Must(c, uuid.NewUUID).String()
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:           secretID,
			ValueRefBackendID:  "backend-123",
			ValueRefRevisionID: "rev-ext-456",
			Checksum:           "checksum-external",
			RevisionUUID:       revUUID,
			OwnerKind:          secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	// Verify revision was created with value_ref
	var gotBackendID, gotRevisionID string
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT backend_uuid, revision_id
FROM   secret_value_ref
WHERE  revision_uuid = ?
`, revUUID).Scan(&gotBackendID, &gotRevisionID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotBackendID, tc.Equals, "backend-123")
	c.Check(gotRevisionID, tc.Equals, "rev-ext-456")

	// Verify no secret_content rows for this revision
	var contentCount int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_content WHERE revision_uuid = ?",
			revUUID).Scan(&contentCount)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(contentCount, tc.Equals, 0)
}

// TestUpdateSecretsChecksumDeduplication verifies that updating a secret
// always creates a new revision, even when the checksum matches the current
// one. This prevents TOCTOU races between the checksum pre-check and the
// model-DB transaction.
func (s *commitHookSuite) TestUpdateSecretsChecksumDeduplication(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-checksum-dedup"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	s.addSecretRevision(c, secretID, 1)

	data := map[string]string{"key": "value"}
	checksum := "same-checksum"

	revUUID1 := tc.Must(c, uuid.NewUUID).String()
	arg1 := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:     secretID,
			Data:         data,
			Checksum:     checksum,
			RevisionUUID: revUUID1,
			OwnerKind:    secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg1), tc.ErrorIsNil)

	var revCountAfterFirst int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_revision WHERE secret_id = ?",
			secretID).Scan(&revCountAfterFirst)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(revCountAfterFirst, tc.Equals, 2)

	revUUID2 := tc.Must(c, uuid.NewUUID).String()
	arg2 := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:     secretID,
			Data:         data,
			Checksum:     checksum,
			RevisionUUID: revUUID2,
			OwnerKind:    secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg2), tc.ErrorIsNil)

	var revCountAfterSecond int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_revision WHERE secret_id = ?",
			secretID).Scan(&revCountAfterSecond)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(revCountAfterSecond, tc.Equals, 3)

	var latestChecksum string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT latest_revision_checksum FROM secret_metadata WHERE secret_id = ?",
			secretID).Scan(&latestChecksum)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(latestChecksum, tc.Equals, checksum)

	var pendingDeleteForRev2 bool
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		scanErr := tx.QueryRowContext(ctx, `
SELECT COALESCE(pending_delete, false)
FROM   secret_revision_obsolete ro
JOIN   secret_revision sr ON sr.uuid = ro.revision_uuid
WHERE  sr.secret_id = ? AND sr.revision = 2
`, secretID).Scan(&pendingDeleteForRev2)
		if scanErr == sql.ErrNoRows {
			return nil
		}
		return scanErr
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(pendingDeleteForRev2, tc.IsTrue)
}

func (s *commitHookSuite) TestUpdateSecretsDifferentChecksumCreatesNewRevision(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-different-checksum"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	s.addSecretRevision(c, secretID, 1)

	revUUID1 := tc.Must(c, uuid.NewUUID).String()
	arg1 := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:     secretID,
			Data:         map[string]string{"key": "value1"},
			Checksum:     "checksum-1",
			RevisionUUID: revUUID1,
			OwnerKind:    secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg1), tc.ErrorIsNil)

	revUUID2 := tc.Must(c, uuid.NewUUID).String()
	arg2 := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:     secretID,
			Data:         map[string]string{"key": "value2"},
			Checksum:     "checksum-2",
			RevisionUUID: revUUID2,
			OwnerKind:    secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg2), tc.ErrorIsNil)

	var revCount int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_revision WHERE secret_id = ?",
			secretID).Scan(&revCount)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(revCount, tc.Equals, 3)
}

func (s *commitHookSuite) TestUpdateSecretsRotatePolicyMoreFrequent(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-rotate-freq"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	s.addSecretRevision(c, secretID, 1)

	oldNextRotate := time.Now().UTC().Truncate(time.Microsecond).Add(24 * time.Hour)
	s.query(c,
		"INSERT INTO secret_rotation (secret_id, next_rotation_time) VALUES (?, ?)",
		secretID, oldNextRotate)

	policy := secret.RotateHourly
	newNextRotate := time.Now().UTC().Truncate(time.Microsecond).Add(time.Hour)
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:       secretID,
			RotatePolicy:   &policy,
			NextRotateTime: &newNextRotate,
			Checksum:       "checksum-rotate-freq",
			OwnerKind:      secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var gotPolicyID int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT rotate_policy_id FROM secret_metadata WHERE secret_id = ?",
			secretID).Scan(&gotPolicyID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotPolicyID, tc.Equals, int(policy))

	var gotNextRotate time.Time
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT next_rotation_time FROM secret_rotation WHERE secret_id = ?",
			secretID).Scan(&gotNextRotate)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotNextRotate.UTC(), tc.Equals, newNextRotate.UTC())
}

func (s *commitHookSuite) TestUpdateSecretsRotatePolicyToNever(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-rotate-never"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	s.addSecretRevision(c, secretID, 1)

	nextRotate := time.Now().UTC().Truncate(time.Microsecond).Add(time.Hour)
	s.query(c,
		"INSERT INTO secret_rotation (secret_id, next_rotation_time) VALUES (?, ?)",
		secretID, nextRotate)

	policy := secret.RotateNever
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:     secretID,
			RotatePolicy: &policy,
			Checksum:     "checksum-rotate-never",
			OwnerKind:    secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var gotPolicyID int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT rotate_policy_id FROM secret_metadata WHERE secret_id = ?",
			secretID).Scan(&gotPolicyID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotPolicyID, tc.Equals, int(policy))

	var rotationCount int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_rotation WHERE secret_id = ?",
			secretID).Scan(&rotationCount)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(rotationCount, tc.Equals, 0)
}

func (s *commitHookSuite) TestUpdateSecretsRotatePolicyLessFrequentKeepsExistingRotation(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-rotate-less"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	s.addSecretRevision(c, secretID, 1)

	oldNextRotate := time.Now().UTC().Truncate(time.Microsecond).Add(time.Hour)
	s.query(c,
		"INSERT INTO secret_rotation (secret_id, next_rotation_time) VALUES (?, ?)",
		secretID, oldNextRotate)

	policy := secret.RotateDaily
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:     secretID,
			RotatePolicy: &policy,
			Checksum:     "checksum-rotate-less",
			OwnerKind:    secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var gotPolicyID int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT rotate_policy_id FROM secret_metadata WHERE secret_id = ?",
			secretID).Scan(&gotPolicyID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotPolicyID, tc.Equals, int(policy))

	var gotNextRotate time.Time
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT next_rotation_time FROM secret_rotation WHERE secret_id = ?",
			secretID).Scan(&gotNextRotate)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotNextRotate.UTC(), tc.Equals, oldNextRotate.UTC())
}

// TestUpdateSecretsWithExpireTimeOnly verifies that updating a secret with
// only ExpireTime (no new data, no new value ref) applies the expiry to the
// existing latest revision. This is Gap 2 from PR#22651 followup.
func (s *commitHookSuite) TestUpdateSecretsWithExpireTimeOnly(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-expire-only"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	existingRevUUID := s.addSecretRevision(c, secretID, 1)

	expireTime := time.Now().UTC().Truncate(time.Microsecond).Add(48 * time.Hour)
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:   secretID,
			ExpireTime: &expireTime,
			Checksum:   "checksum-expire-only",
			OwnerKind:  secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	// Verify that no new revision was created
	var revCount int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_revision WHERE secret_id = ?",
			secretID).Scan(&revCount)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(revCount, tc.Equals, 1)

	// Verify that expiry was applied to the existing revision
	var gotExpire time.Time
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT expire_time FROM secret_revision_expire WHERE revision_uuid = ?",
			existingRevUUID).Scan(&gotExpire)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotExpire.UTC(), tc.Equals, expireTime.UTC())
}

// TestUpdateSecretsWithExpireTimeAndNewData verifies that updating a secret
// with both ExpireTime and new data creates a new revision and applies the
// expiry to that new revision (regression guard).
func (s *commitHookSuite) TestUpdateSecretsWithExpireTimeAndNewData(c *tc.C) {
	ctx := c.Context()
	secretID := "update-test-expire-with-data"
	s.addSecretWithOwner(c, secretID, s.unitUUID, "unit")
	s.addSecretRevision(c, secretID, 1)

	revUUID := tc.Must(c, uuid.NewUUID).String()
	data := map[string]string{"key1": "newvalue1"}
	expireTime := time.Now().UTC().Truncate(time.Microsecond).Add(24 * time.Hour)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:     secretID,
			Data:         data,
			ExpireTime:   &expireTime,
			Checksum:     "checksum-expire-with-data",
			RevisionUUID: revUUID,
			OwnerKind:    secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	// Verify that new revision was created
	var revCount int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_revision WHERE secret_id = ?",
			secretID).Scan(&revCount)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(revCount, tc.Equals, 2)

	// Verify that expiry was applied to the new revision
	var gotExpire time.Time
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT expire_time FROM secret_revision_expire WHERE revision_uuid = ?",
			revUUID).Scan(&gotExpire)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotExpire.UTC(), tc.Equals, expireTime.UTC())

	// Verify data was stored in the new revision
	var contentCount int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT count(*) FROM secret_content WHERE revision_uuid = ?",
			revUUID).Scan(&contentCount)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(contentCount, tc.Equals, 1)
}

// addSecretWithOwner inserts a secret with an owner row.
func (s *commitHookSuite) addSecretWithOwner(c *tc.C, secretID, ownerUUID, ownerKind string) {
	s.query(c, `INSERT INTO secret (id) VALUES (?)`, secretID)
	s.query(c, `INSERT INTO secret_metadata (secret_id, version, rotate_policy_id, create_time, update_time) VALUES (?, 1, 0, DATETIME('now', 'utc'), DATETIME('now', 'utc'))`, secretID)
	switch ownerKind {
	case "unit":
		s.query(c, `INSERT INTO secret_unit_owner (secret_id, unit_uuid) VALUES (?, ?)`, secretID, ownerUUID)
	case "application":
		s.query(c, `INSERT INTO secret_application_owner (secret_id, application_uuid) VALUES (?, ?)`, secretID, ownerUUID)
	}
}

func (s *commitHookSuite) TestCreateSecretsUnitOwned(c *tc.C) {
	ctx := c.Context()
	secretID := "create-test-unit"
	revUUID := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  secretID,
			Version:   1,
			OwnerKind: secret.UnitCharmSecretOwner,
			OwnerUUID: s.unitUUID,
			Label:     "my-unit-secret",
			Params: secret.UpsertSecretParams{
				RevisionUUID: &revUUID,
				CreateTime:   now,
				UpdateTime:   now,
				Data:         coresecrets.SecretData{"key": "val"},
				Checksum:     "checksum-unit",
			},
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var count int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT count(*) FROM secret WHERE id = ?", secretID).Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1)

	var label string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT label FROM secret_unit_owner WHERE secret_id = ?", secretID).Scan(&label)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(label, tc.Equals, "my-unit-secret")

	var roleID int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT role_id FROM secret_permission WHERE secret_id = ? AND subject_uuid = ?", secretID, s.unitUUID).Scan(&roleID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(roleID, tc.Equals, int(secret.RoleManage))
}

func (s *commitHookSuite) TestCreateSecretsApplicationOwned(c *tc.C) {
	ctx := c.Context()
	secretID := "create-test-app"
	revUUID := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  secretID,
			Version:   1,
			OwnerKind: secret.ApplicationCharmSecretOwner,
			OwnerUUID: s.fakeApplicationUUID1,
			Label:     "my-app-secret",
			Params: secret.UpsertSecretParams{
				RevisionUUID: &revUUID,
				CreateTime:   now,
				UpdateTime:   now,
				Data:         coresecrets.SecretData{"key": "val"},
				Checksum:     "checksum-app",
			},
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var label string
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT label FROM secret_application_owner WHERE secret_id = ?", secretID).Scan(&label)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(label, tc.Equals, "my-app-secret")

	var roleID int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT role_id FROM secret_permission WHERE secret_id = ? AND subject_uuid = ?", secretID, s.fakeApplicationUUID1).Scan(&roleID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(roleID, tc.Equals, int(secret.RoleManage))
}

func (s *commitHookSuite) TestCreateSecretsWithRotatePolicy(c *tc.C) {
	ctx := c.Context()
	secretID := "create-test-rotate"
	revUUID := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)
	nextRotate := now.Add(24 * time.Hour)

	policy := secret.RotateDaily
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  secretID,
			Version:   1,
			OwnerKind: secret.UnitCharmSecretOwner,
			OwnerUUID: s.unitUUID,
			Params: secret.UpsertSecretParams{
				RevisionUUID:   &revUUID,
				RotatePolicy:   &policy,
				NextRotateTime: &nextRotate,
				CreateTime:     now,
				UpdateTime:     now,
				Checksum:       "checksum-rotate",
			},
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var gotPolicyID int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT rotate_policy_id FROM secret_metadata WHERE secret_id = ?", secretID).Scan(&gotPolicyID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotPolicyID, tc.Equals, int(policy))

	var gotNextRotate time.Time
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT next_rotation_time FROM secret_rotation WHERE secret_id = ?", secretID).Scan(&gotNextRotate)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotNextRotate.UTC(), tc.Equals, nextRotate.UTC())
}

func (s *commitHookSuite) TestCreateSecretsWithExpireTime(c *tc.C) {
	ctx := c.Context()
	secretID := "create-test-expire"
	revUUID := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)
	expireTime := now.Add(48 * time.Hour)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  secretID,
			Version:   1,
			OwnerKind: secret.UnitCharmSecretOwner,
			OwnerUUID: s.unitUUID,
			Params: secret.UpsertSecretParams{
				RevisionUUID: &revUUID,
				ExpireTime:   &expireTime,
				CreateTime:   now,
				UpdateTime:   now,
				Checksum:     "checksum-expire",
			},
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var gotExpire time.Time
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT expire_time FROM secret_revision_expire WHERE revision_uuid = ?", revUUID).Scan(&gotExpire)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotExpire.UTC(), tc.Equals, expireTime.UTC())
}

func (s *commitHookSuite) TestCreateSecretsWithValueRef(c *tc.C) {
	ctx := c.Context()
	secretID := "create-test-valref"
	revUUID := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  secretID,
			Version:   1,
			OwnerKind: secret.UnitCharmSecretOwner,
			OwnerUUID: s.unitUUID,
			Params: secret.UpsertSecretParams{
				RevisionUUID: &revUUID,
				CreateTime:   now,
				UpdateTime:   now,
				ValueRef: &coresecrets.ValueRef{
					BackendID:  "backend-uuid",
					RevisionID: "ext-rev-123",
				},
				Checksum: "checksum-valref",
			},
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var backendUUID, revisionID string
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT backend_uuid, revision_id FROM secret_value_ref WHERE revision_uuid = ?", revUUID).Scan(&backendUUID, &revisionID)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(backendUUID, tc.Equals, "backend-uuid")
	c.Check(revisionID, tc.Equals, "ext-rev-123")
}

func (s *commitHookSuite) TestCreateSecretsMultiple(c *tc.C) {
	ctx := c.Context()
	revUUID1 := tc.Must(c, uuid.NewUUID).String()
	revUUID2 := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{
			{
				SecretID:  "create-multi-1",
				Version:   1,
				OwnerKind: secret.UnitCharmSecretOwner,
				OwnerUUID: s.unitUUID,
				Label:     "secret-one",
				Params: secret.UpsertSecretParams{
					RevisionUUID: &revUUID1,
					CreateTime:   now,
					UpdateTime:   now,
					Data:         coresecrets.SecretData{"a": "1"},
					Checksum:     "checksum-1",
				},
			},
			{
				SecretID:  "create-multi-2",
				Version:   1,
				OwnerKind: secret.ApplicationCharmSecretOwner,
				OwnerUUID: s.fakeApplicationUUID1,
				Label:     "secret-two",
				Params: secret.UpsertSecretParams{
					RevisionUUID: &revUUID2,
					CreateTime:   now,
					UpdateTime:   now,
					Data:         coresecrets.SecretData{"b": "2"},
					Checksum:     "checksum-2",
				},
			},
		},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var count int
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT count(*) FROM secret WHERE id IN ('create-multi-1', 'create-multi-2')").Scan(&count)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 2)

	var label1, label2 string
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT label FROM secret_unit_owner WHERE secret_id = ?", "create-multi-1").Scan(&label1); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, "SELECT label FROM secret_application_owner WHERE secret_id = ?", "create-multi-2").Scan(&label2)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(label1, tc.Equals, "secret-one")
	c.Check(label2, tc.Equals, "secret-two")
}

func (s *commitHookSuite) TestCreateSecretsWithLabel(c *tc.C) {
	ctx := c.Context()
	secretID := "create-test-label"
	revUUID := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  secretID,
			Version:   1,
			OwnerKind: secret.UnitCharmSecretOwner,
			OwnerUUID: s.unitUUID,
			Label:     "my-labeled-secret",
			Params: secret.UpsertSecretParams{
				RevisionUUID: &revUUID,
				CreateTime:   now,
				UpdateTime:   now,
				Data:         coresecrets.SecretData{"key": "val"},
				Checksum:     "checksum-label",
			},
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var label string
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT label FROM secret_unit_owner WHERE secret_id = ?", secretID).Scan(&label)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(label, tc.Equals, "my-labeled-secret")
}

func (s *commitHookSuite) TestCreateSecretsNoRevisionUUIDFails(c *tc.C) {
	ctx := c.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  "create-no-revuuid",
			Version:   1,
			OwnerKind: secret.UnitCharmSecretOwner,
			OwnerUUID: s.unitUUID,
			Params: secret.UpsertSecretParams{
				CreateTime: now,
				UpdateTime: now,
				Checksum:   "checksum",
			},
		}},
	}

	err := s.state.CommitHookChanges(ctx, arg)
	c.Check(err, tc.ErrorMatches, `.*revision UUID must be provided.*`)
}

func (s *commitHookSuite) TestCreateSecretIDConflict(c *tc.C) {
	ctx := c.Context()
	secretID := "create-test-conflict"
	revUUID := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s.addSecret(c, secretID)

	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  secretID,
			Version:   1,
			OwnerKind: secret.UnitCharmSecretOwner,
			OwnerUUID: s.unitUUID,
			Params: secret.UpsertSecretParams{
				RevisionUUID: &revUUID,
				CreateTime:   now,
				UpdateTime:   now,
				Checksum:     "checksum",
			},
		}},
	}

	err := s.state.CommitHookChanges(ctx, arg)
	c.Check(err, tc.Not(tc.IsNil))
	c.Check(err, tc.ErrorMatches, `create secrets: creating secret "create-test-conflict": secret already exists`)
}

func (s *commitHookSuite) TestCreateSecretsRollsBackOnStorageFailure(c *tc.C) {
	ctx := c.Context()
	secretID := "create-test-rollback"
	revUUID := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)

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
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  secretID,
			Version:   1,
			OwnerKind: secret.UnitCharmSecretOwner,
			OwnerUUID: s.unitUUID,
			Params: secret.UpsertSecretParams{
				RevisionUUID: &revUUID,
				CreateTime:   now,
				UpdateTime:   now,
				Data:         coresecrets.SecretData{"key": "val"},
				Checksum:     "checksum-rollback",
			},
		}},
		AddStorage: []unitstate.PreparedStorageAdd{{
			StorageName: "data",
			Storage: domainstorage.IAASUnitAddStorageArg{
				UnitAddStorageArg: domainstorage.UnitAddStorageArg{
					CountLessThanEqual: 0,
				},
			},
		}},
	}

	err := s.state.CommitHookChanges(ctx, arg)
	c.Assert(err, tc.ErrorIs, storageerrors.MaxStorageCountPreconditionFailed)

	var secretCount int
	err = s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT count(*) FROM secret WHERE id = ?", secretID).Scan(&secretCount)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(secretCount, tc.Equals, 0)
}

// TestCreateSecretsLabelConflictApplication verifies that creating an
// application-owned secret with a label already used by another
// application-owned secret of the same application returns
// SecretLabelAlreadyExists.
func (s *commitHookSuite) TestCreateSecretsLabelConflictApplication(c *tc.C) {
	ctx := c.Context()
	existingID := "existing-app-label-conflict"
	s.addSecretWithOwner(c, existingID, s.fakeApplicationUUID1, "application")
	s.query(c, `UPDATE secret_application_owner SET label = ? WHERE secret_id = ?`,
		"dup-app-label", existingID)

	revUUID := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  "new-app-label-conflict",
			Version:   1,
			OwnerKind: secret.ApplicationCharmSecretOwner,
			OwnerUUID: s.fakeApplicationUUID1,
			Label:     "dup-app-label",
			Params: secret.UpsertSecretParams{
				RevisionUUID: &revUUID,
				CreateTime:   now,
				UpdateTime:   now,
				Data:         coresecrets.SecretData{"key": "val"},
				Checksum:     "checksum",
			},
		}},
	}

	err := s.state.CommitHookChanges(ctx, arg)
	c.Check(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

// TestCreateSecretsLabelConflictUnit verifies that creating a unit-owned
// secret with a label already used by another unit-owned secret of the same
// unit returns SecretLabelAlreadyExists.
func (s *commitHookSuite) TestCreateSecretsLabelConflictUnit(c *tc.C) {
	ctx := c.Context()
	existingID := "existing-unit-label-conflict"
	s.addSecretWithOwner(c, existingID, s.unitUUID, "unit")
	s.query(c, `UPDATE secret_unit_owner SET label = ? WHERE secret_id = ?`,
		"dup-unit-label", existingID)

	revUUID := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  "new-unit-label-conflict",
			Version:   1,
			OwnerKind: secret.UnitCharmSecretOwner,
			OwnerUUID: s.unitUUID,
			Label:     "dup-unit-label",
			Params: secret.UpsertSecretParams{
				RevisionUUID: &revUUID,
				CreateTime:   now,
				UpdateTime:   now,
				Data:         coresecrets.SecretData{"key": "val"},
				Checksum:     "checksum",
			},
		}},
	}

	err := s.state.CommitHookChanges(ctx, arg)
	c.Check(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

// TestCreateSecretsLabelConflictCrossKind verifies that a label conflict is
// detected across owner kinds: a unit-owned secret's label conflicts with a
// new application-owned secret of the same application, and vice versa.
func (s *commitHookSuite) TestCreateSecretsLabelConflictCrossKind(c *tc.C) {
	ctx := c.Context()

	// Create a unit under fakeApplicationUUID1 so the cross-kind join
	// (unit → application) can find the conflict.
	crossUnitUUID := s.addUnitAndNetNode(c, "cross-app/0",
		s.fakeApplicationUUID1, s.fakeCharmUUID1).String()

	// Seed a unit-owned secret with label "cross-label".
	existingID := "existing-cross-label"
	s.addSecretWithOwner(c, existingID, crossUnitUUID, "unit")
	s.query(c, `UPDATE secret_unit_owner SET label = ? WHERE secret_id = ?`,
		"cross-label", existingID)

	// Attempt to create an application-owned secret with the same label
	// for the application that owns the unit.
	revUUID := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  "new-cross-label",
			Version:   1,
			OwnerKind: secret.ApplicationCharmSecretOwner,
			OwnerUUID: s.fakeApplicationUUID1,
			Label:     "cross-label",
			Params: secret.UpsertSecretParams{
				RevisionUUID: &revUUID,
				CreateTime:   now,
				UpdateTime:   now,
				Data:         coresecrets.SecretData{"key": "val"},
				Checksum:     "checksum",
			},
		}},
	}

	err := s.state.CommitHookChanges(ctx, arg)
	c.Check(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

// TestCreateSecretsLabelNoConflictDifferentOwner verifies that the same label
// can be used by secrets owned by different owners without error.
func (s *commitHookSuite) TestCreateSecretsLabelNoConflictDifferentOwner(c *tc.C) {
	ctx := c.Context()
	existingID := "existing-diff-owner-label"
	s.addSecretWithOwner(c, existingID, s.fakeApplicationUUID1, "application")
	s.query(c, `UPDATE secret_application_owner SET label = ? WHERE secret_id = ?`,
		"shared-label", existingID)

	revUUID := tc.Must(c, uuid.NewUUID).String()
	now := time.Now().UTC().Truncate(time.Microsecond)
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretCreates: []internal.CreateSecretArg{{
			SecretID:  "new-diff-owner-label",
			Version:   1,
			OwnerKind: secret.ApplicationCharmSecretOwner,
			OwnerUUID: s.fakeApplicationUUID2,
			Label:     "shared-label",
			Params: secret.UpsertSecretParams{
				RevisionUUID: &revUUID,
				CreateTime:   now,
				UpdateTime:   now,
				Data:         coresecrets.SecretData{"key": "val"},
				Checksum:     "checksum",
			},
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)
}

// TestUpdateSecretsLabelConflict verifies that updating a secret's label to
// one already used by another secret of the same owner returns
// SecretLabelAlreadyExists.
func (s *commitHookSuite) TestUpdateSecretsLabelConflict(c *tc.C) {
	ctx := c.Context()

	// Seed two unit-owned secrets for the same unit; the first has label
	// "taken-label".
	secretA := "update-label-conflict-a"
	secretB := "update-label-conflict-b"
	s.addSecretWithOwner(c, secretA, s.unitUUID, "unit")
	s.addSecretRevision(c, secretA, 1)
	s.query(c, `UPDATE secret_unit_owner SET label = ? WHERE secret_id = ?`,
		"taken-label", secretA)

	s.addSecretWithOwner(c, secretB, s.unitUUID, "unit")
	s.addSecretRevision(c, secretB, 1)

	// Attempt to update secretB's label to "taken-label".
	newLabel := "taken-label"
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:  secretB,
			Label:     &newLabel,
			OwnerKind: secret.UnitCharmSecretOwner,
		}},
	}

	err := s.state.CommitHookChanges(ctx, arg)
	c.Check(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
}

// TestUpdateSecretsLabelNoConflict verifies that updating a secret's label to
// a unique value succeeds.
func (s *commitHookSuite) TestUpdateSecretsLabelNoConflict(c *tc.C) {
	ctx := c.Context()

	secretA := "update-label-noconflict-a"
	secretB := "update-label-noconflict-b"
	s.addSecretWithOwner(c, secretA, s.unitUUID, "unit")
	s.addSecretRevision(c, secretA, 1)
	s.query(c, `UPDATE secret_unit_owner SET label = ? WHERE secret_id = ?`,
		"existing-label", secretA)

	s.addSecretWithOwner(c, secretB, s.unitUUID, "unit")
	s.addSecretRevision(c, secretB, 1)

	newLabel := "unique-label"
	arg := internal.CommitHookChangesArg{
		UnitUUID: s.unitUUID,
		SecretUpdates: []internal.UpdateSecretArg{{
			SecretID:  secretB,
			Label:     &newLabel,
			OwnerKind: secret.UnitCharmSecretOwner,
		}},
	}

	c.Assert(s.state.CommitHookChanges(ctx, arg), tc.ErrorIsNil)

	var gotLabel string
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			"SELECT label FROM secret_unit_owner WHERE secret_id = ?",
			secretB).Scan(&gotLabel)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(gotLabel, tc.Equals, newLabel)
}
