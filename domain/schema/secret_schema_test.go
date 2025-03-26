// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"

	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/errors"
)

type secretSchemaSuite struct {
	schemaBaseSuite
}

var _ = gc.Suite(&secretSchemaSuite{})

func (s *secretSchemaSuite) TestControllerChangeLogTriggersForSecretBackends(c *gc.C) {
	s.applyDDL(c, ControllerDDL())

	s.assertChangeLogCount(c, 1, tableSecretBackendRotation, 0)
	s.assertChangeLogCount(c, 2, tableSecretBackendRotation, 0)
	s.assertChangeLogCount(c, 4, tableSecretBackendRotation, 0)

	backendUUID := utils.MustNewUUID().String()

	s.assertExecSQL(c, "INSERT INTO secret_backend (uuid, name, backend_type_id) VALUES (?, 'myVault', 2);", backendUUID)
	s.assertExecSQL(c, "INSERT INTO secret_backend_rotation (backend_uuid, next_rotation_time) VALUES (?, datetime('now', '+1 day'));", backendUUID)
	s.assertExecSQL(c, `UPDATE secret_backend_rotation SET next_rotation_time = datetime('now', '+2 day') WHERE backend_uuid = ?;`, backendUUID)
	s.assertExecSQL(c, `DELETE FROM secret_backend_rotation WHERE backend_uuid = ?;`, backendUUID)

	s.assertChangeLogCount(c, 1, tableSecretBackendRotation, 1)
	s.assertChangeLogCount(c, 2, tableSecretBackendRotation, 1)
	s.assertChangeLogCount(c, 4, tableSecretBackendRotation, 1)
}

func (s *secretSchemaSuite) TestModelChangeLogTriggersForSecretTables(c *gc.C) {
	s.applyDDL(c, ModelDDL())

	// secret table triggers.
	s.assertChangeLogCount(c, 1, tableSecretMetadataAutoPrune, 0)
	s.assertChangeLogCount(c, 2, tableSecretMetadataAutoPrune, 0)
	s.assertChangeLogCount(c, 4, tableSecretMetadataAutoPrune, 0)

	secretURI := coresecrets.NewURI()
	s.assertExecSQL(c, `INSERT INTO secret (id) VALUES (?);`, secretURI.ID)
	s.assertExecSQL(c, `INSERT INTO secret_metadata (secret_id, version, description, rotate_policy_id) VALUES (?, 1, 'mySecret', 0);`, secretURI.ID)
	s.assertExecSQL(c, `UPDATE secret_metadata SET auto_prune = true WHERE secret_id = ?;`, secretURI.ID)
	s.assertExecSQL(c, `DELETE FROM secret_metadata WHERE secret_id = ?;`, secretURI.ID)

	s.assertChangeLogCount(c, 1, tableSecretMetadataAutoPrune, 1)
	s.assertChangeLogCount(c, 2, tableSecretMetadataAutoPrune, 1)
	s.assertChangeLogCount(c, 4, tableSecretMetadataAutoPrune, 1)

	// secret_rotation table triggers.
	s.assertChangeLogCount(c, 1, tableSecretRotation, 0)
	s.assertChangeLogCount(c, 2, tableSecretRotation, 0)
	s.assertChangeLogCount(c, 4, tableSecretRotation, 0)

	s.assertExecSQL(c, `INSERT INTO secret_metadata (secret_id, version, description, rotate_policy_id) VALUES (?, 1, 'mySecret', 0);`, secretURI.ID)
	s.assertExecSQL(c, `INSERT INTO secret_rotation (secret_id, next_rotation_time) VALUES (?, datetime('now', '+1 day'));`, secretURI.ID)
	s.assertExecSQL(c, `UPDATE secret_rotation SET next_rotation_time = datetime('now', '+2 day') WHERE secret_id = ?;`, secretURI.ID)
	s.assertExecSQL(c, `DELETE FROM secret_rotation WHERE secret_id = ?;`, secretURI.ID)

	s.assertChangeLogCount(c, 1, tableSecretRotation, 1)
	s.assertChangeLogCount(c, 2, tableSecretRotation, 1)
	s.assertChangeLogCount(c, 4, tableSecretRotation, 1)

	// secret_revision table triggers.
	revisionUUID := utils.MustNewUUID().String()
	s.assertChangeLogCount(c, 1, tableSecretRevisionObsolete, 0)
	s.assertChangeLogCount(c, 2, tableSecretRevisionObsolete, 0)
	s.assertChangeLogCount(c, 4, tableSecretRevisionObsolete, 0)

	s.assertExecSQL(c, `INSERT INTO secret_revision (uuid, secret_id, revision) VALUES (?, ?, 1);`, revisionUUID, secretURI.ID)
	s.assertExecSQL(c, `INSERT INTO secret_revision_obsolete (revision_uuid) VALUES (?);`, revisionUUID)
	s.assertExecSQL(c, `UPDATE secret_revision_obsolete SET obsolete = true WHERE revision_uuid = ?;`, revisionUUID)
	s.assertExecSQL(c, `DELETE FROM secret_revision_obsolete WHERE revision_uuid = ?;`, revisionUUID)
	s.assertExecSQL(c, `DELETE FROM secret_revision WHERE uuid = ?;`, revisionUUID)

	s.assertChangeLogCount(c, 1, tableSecretRevisionObsolete, 1)
	s.assertChangeLogCount(c, 2, tableSecretRevisionObsolete, 1)
	s.assertChangeLogCount(c, 4, tableSecretRevisionObsolete, 1)

	// secret_revision_expire table triggers.
	s.assertChangeLogCount(c, 1, tableSecretRevisionExpire, 0)
	s.assertChangeLogCount(c, 2, tableSecretRevisionExpire, 0)
	s.assertChangeLogCount(c, 4, tableSecretRevisionExpire, 0)

	s.assertExecSQL(c, `INSERT INTO secret_revision (uuid, secret_id, revision) VALUES (?, ?, 1);`, revisionUUID, secretURI.ID)
	s.assertExecSQL(c, `INSERT INTO secret_revision_expire (revision_uuid, expire_time) VALUES (?, datetime('now', '+1 day'));`, revisionUUID)
	s.assertExecSQL(c, `UPDATE secret_revision_expire SET expire_time = datetime('now', '+2 day') WHERE revision_uuid = ?;`, revisionUUID)
	s.assertExecSQL(c, `DELETE FROM secret_revision_expire WHERE revision_uuid = ?;`, revisionUUID)
	s.assertExecSQL(c, `DELETE FROM secret_revision WHERE uuid = ?;`, revisionUUID)

	s.assertChangeLogCount(c, 1, tableSecretRevisionExpire, 1)
	s.assertChangeLogCount(c, 2, tableSecretRevisionExpire, 1)
	s.assertChangeLogCount(c, 4, tableSecretRevisionExpire, 1)

	// secret_revision table triggers.
	s.assertChangeLogCount(c, 1, tableSecretRevision, 2)
	s.assertChangeLogCount(c, 2, tableSecretRevision, 0)
	s.assertChangeLogCount(c, 4, tableSecretRevision, 2)

	s.assertExecSQL(c, `INSERT INTO secret_revision (uuid, secret_id, revision) VALUES (?, ?, 1);`, revisionUUID, secretURI.ID)
	s.assertExecSQL(c, `DELETE FROM secret_revision WHERE uuid = ?;`, revisionUUID)

	s.assertChangeLogCount(c, 1, tableSecretRevision, 3)
	s.assertChangeLogCount(c, 2, tableSecretRevision, 0)
	s.assertChangeLogCount(c, 4, tableSecretRevision, 3)

	// secret_reference table triggers.
	s.assertChangeLogCount(c, 1, tableSecretReference, 0)
	s.assertChangeLogCount(c, 2, tableSecretReference, 0)
	s.assertChangeLogCount(c, 4, tableSecretReference, 0)

	s.assertExecSQL(c, `INSERT INTO secret_reference (secret_id, latest_revision) VALUES (?, 1);`, secretURI.ID)
	s.assertExecSQL(c, `UPDATE secret_reference SET latest_revision = 2 WHERE secret_id = ?;`, secretURI.ID)
	s.assertExecSQL(c, `DELETE FROM secret_reference WHERE secret_id = ?;`, secretURI.ID)

	s.assertChangeLogCount(c, 1, tableSecretReference, 1)
	s.assertChangeLogCount(c, 2, tableSecretReference, 1)
	s.assertChangeLogCount(c, 4, tableSecretReference, 1)

	// secret_deleted_value_ref table triggers.
	deletedRvisionUUID := utils.MustNewUUID().String()
	backendUUIDUUID := utils.MustNewUUID().String()
	s.assertChangeLogCount(c, 1, tableSecretDeletedValueRef, 0)
	s.assertChangeLogCount(c, 2, tableSecretDeletedValueRef, 0)
	s.assertChangeLogCount(c, 4, tableSecretDeletedValueRef, 0)

	// We only care about inserts into this table.
	s.assertExecSQL(c, `INSERT INTO secret_deleted_value_ref (revision_uuid, backend_uuid, revision_id) VALUES (?, ?, ?);`, deletedRvisionUUID, backendUUIDUUID, "rev")
	s.assertChangeLogCount(c, 1, tableSecretDeletedValueRef, 1)

	charmUUID := utils.MustNewUUID().String()
	s.assertExecSQL(c, "INSERT INTO charm (uuid, reference_name, source_id, architecture_id) VALUES (?, 'mysql', 0, 0);", charmUUID)
	s.assertExecSQL(c, "INSERT INTO charm_metadata (charm_uuid, name) VALUES (?, 'mysql');", charmUUID)

	appUUID := utils.MustNewUUID().String()
	s.assertExecSQL(c, `
INSERT INTO application (uuid, charm_uuid, name, life_id, password_hash_algorithm_id, password_hash, space_uuid)
VALUES (?, ?, 'mysql', 0, 0, 'K68fQBBdlQH+MZqOxGP99DJaKl30Ra3z9XL2JiU2eMk=', ?);`, appUUID, charmUUID, network.AlphaSpaceId)

	unitNetNodeUUID := utils.MustNewUUID().String()
	s.assertExecSQL(c, `INSERT INTO net_node (uuid) VALUES (?);`, unitNetNodeUUID)
	unitUUID := utils.MustNewUUID().String()
	s.assertExecSQL(c, `
INSERT INTO unit (uuid, life_id, name, application_uuid, net_node_uuid, charm_uuid, resolve_kind_id)
VALUES (?, 0, 0, ?, ?, ?, 0);`,
		unitUUID, appUUID, unitNetNodeUUID, charmUUID)
}

func (s *secretSchemaSuite) assertChangeLogCount(c *gc.C, editType int, namespaceID tableNamespaceID, expectedCount int) {
	var count int
	_ = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT COUNT(*) FROM change_log
WHERE edit_type_id = ? AND namespace_id = ?;`[1:], editType, namespaceID)

		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		if !rows.Next() {
			return errors.Errorf("no rows returned")
		}
		return rows.Scan(&count)
	})
	c.Assert(count, gc.Equals, expectedCount)
}
