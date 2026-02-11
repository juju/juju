// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/tc"

	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/internal"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// importSuite is a set of tests to assert the interface and contracts
// importing storage into this state package.
type importSuite struct {
	testing.ModelSuite
}

// TestImportSuite runs all of the tests contained in
// [importSuite].
func TestImportSuite(t *stdtesting.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) TestImportStorageInstances(c *tc.C) {
	// Arrange
	ebsPoolUUID := s.newStoragePool(c, "ebs", "fspool").String()
	gcePoolUUID := s.newStoragePool(c, "gce", "testme").String()
	netNodeUUID := s.newNetNode(c)
	appUUID, _ := s.newApplication(c, "foo")
	unit, unitName := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)
	args := []internal.ImportStorageInstanceArgs{
		{
			UUID:             tc.Must(c, uuid.NewUUID).String(),
			Life:             int(life.Alive),
			StorageName:      "multi-fs",
			StorageKind:      "block",
			StorageID:        "multi-fs/0",
			PoolName:         "ebs",
			RequestedSizeMiB: uint64(1024),
			UnitName:         unitName,
		}, {
			UUID:             tc.Must(c, uuid.NewUUID).String(),
			Life:             int(life.Alive),
			StorageName:      "another-fs",
			StorageKind:      "filesystem",
			StorageID:        "another-fs/2",
			PoolName:         "gce",
			RequestedSizeMiB: uint64(4048),
			UnitName:         unitName,
		}, { // Add a storage_instance without a unit name.
			UUID:             tc.Must(c, uuid.NewUUID).String(),
			Life:             int(life.Alive),
			StorageName:      "test-fs",
			StorageKind:      "filesystem",
			StorageID:        "test-fs/9",
			PoolName:         "gce",
			RequestedSizeMiB: uint64(4048),
		},
	}

	st := NewState(s.TxnRunnerFactory())

	// Act
	err := st.ImportStorageInstances(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtained := s.getStorageInstances(c)
	c.Check(obtained, tc.SameContents, []importStorageInstance{
		{
			UUID:            args[0].UUID,
			CharmName:       "myapp",
			StorageName:     "multi-fs",
			StorageKindID:   0,
			StorageID:       "multi-fs/0",
			LifeID:          0,
			StoragePoolUUID: ebsPoolUUID,
			RequestedSize:   uint64(1024),
		}, {
			UUID:            args[1].UUID,
			CharmName:       "myapp",
			StorageName:     "another-fs",
			StorageKindID:   1,
			StorageID:       "another-fs/2",
			LifeID:          0,
			StoragePoolUUID: gcePoolUUID,
			RequestedSize:   uint64(4048),
		}, { // No unit name results in no charm name.
			UUID:            args[2].UUID,
			StorageName:     "test-fs",
			StorageKindID:   1,
			StorageID:       "test-fs/9",
			LifeID:          0,
			StoragePoolUUID: gcePoolUUID,
			RequestedSize:   uint64(4048),
		},
	})
	s.checkStorageUnitOwner(c, unit, 2)
}

func (s *importSuite) getStorageInstances(c *tc.C) []importStorageInstance {
	var result []importStorageInstance
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(c.Context(), `
SELECT uuid, charm_name, storage_name, storage_kind_id, storage_id, life_id, storage_pool_uuid, requested_size_mib 
FROM storage_instance`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var uuid, charm_name, storageName, storageID, pool string
			var size uint64
			var kind, life int
			if err := rows.Scan(&uuid, &charm_name, &storageName, &kind, &storageID, &life, &pool, &size); err != nil {
				return err
			}
			result = append(result, importStorageInstance{
				UUID:            uuid,
				CharmName:       charm_name,
				StorageName:     storageName,
				StorageKindID:   kind,
				StoragePoolUUID: pool,
				StorageID:       storageID,
				LifeID:          life,
				RequestedSize:   size,
			})
		}
		return rows.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
	return result
}

func (s *importSuite) checkStorageUnitOwner(c *tc.C, unitUUID string, expected int) {
	var count int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM storage_unit_owner WHERE unit_uuid = $1`, unitUUID).Scan(&count)
		if err != nil {
			return errors.Errorf("getting owner count: %w", err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, expected)
}

// newStoragePool creates a new storage pool with name, provider type and attrs.
// It returns the UUID of the new storage pool.
func (s *importSuite) newStoragePool(c *tc.C,
	name string, providerType string,
) storage.StoragePoolUUID {
	spUUID := storagetesting.GenStoragePoolUUID(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_pool (uuid, name, type)
VALUES (?, ?, ?)`, spUUID.String(), name, providerType)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	return spUUID
}

// newNetNode creates a new net node in the model for referencing to storage
// entity attachments. The net node is not associated with any machine or units.
func (s *importSuite) newNetNode(c *tc.C) domainnetwork.NetNodeUUID {
	nodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO net_node VALUES (?)",
		nodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return nodeUUID
}

// newApplication creates a new application in the model returning the uuid of
// the new application.
func (s *importSuite) newApplication(c *tc.C, name string) (string, string) {
	appUUID := tc.Must(c, uuid.NewUUID)

	charmUUID := s.newCharm(c)

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)`, appUUID.String(), charmUUID, name, network.AlphaSpaceId)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return appUUID.String(), charmUUID
}

// newCharm creates a new charm in the model and returns the uuid for it.
func (s *importSuite) newCharm(c *tc.C) string {
	charmUUID := charmtesting.GenCharmID(c)

	err := s.TxnRunner().StdTxn(
		c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)
`,
				charmUUID.String(), "foo",
			)
			if err != nil {
				return err
			}

			_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, 'myapp')
`,
				charmUUID.String(),
			)
			return err
		})
	c.Assert(err, tc.ErrorIsNil)
	return charmUUID.String()
}

// newUnitWithNetNode creates a new unit in the model for the provided
// application uuid. The new unit will use the supplied net node. Returned is
// the new uuid of the unit and the name that was used.
func (s *importSuite) newUnitWithNetNode(
	c *tc.C, unitName, appUUID string, netNodeUUID domainnetwork.NetNodeUUID,
) (string, string) {
	var charmUUID string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT charm_uuid FROM application WHERE uuid = ?",
		appUUID,
	).Scan(&charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	unit := tc.Must(c, uuid.NewUUID).String()

	_, err = s.DB().ExecContext(
		c.Context(), `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)
`,
		unit, unitName, appUUID, charmUUID, netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return unit, unitName
}
