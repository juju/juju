// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	_ "github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/database/schema"
	databasetesting "github.com/juju/juju/internal/database/testing"
)

type schemaSuite struct {
	databasetesting.DqliteSuite
}

var _ = gc.Suite(&schemaSuite{})

// NewCleanDB returns a new sql.DB reference.
func (s *schemaSuite) NewCleanDB(c *gc.C) *sql.DB {
	dir := c.MkDir()

	url := fmt.Sprintf("file:%s/db.sqlite3?_foreign_keys=1", dir)
	c.Logf("Opening sqlite3 db with: %v", url)

	db, err := sql.Open("sqlite3", url)
	c.Assert(err, jc.ErrorIsNil)

	return db
}

var (
	internalTableNames = set.NewStrings(
		"schema",
		"sqlite_sequence",
	)
)

func readEntityNames(c *gc.C, db *sql.DB, entity_type string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	c.Assert(err, jc.ErrorIsNil)

	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT name FROM sqlite_master WHERE type = ? ORDER BY name ASC;`, entity_type)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		c.Assert(err, jc.ErrorIsNil)
		names = append(names, name)
	}

	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)

	return names
}

func (s *schemaSuite) applyDDL(c *gc.C, ddl *schema.Schema) {
	ddl.Hook(func(i int) error {
		c.Log("Applying schema change", i)
		return nil
	})
	changeSet, err := ddl.Ensure(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(changeSet.Current, gc.Equals, 0)
	c.Check(changeSet.Post, gc.Equals, ddl.Len())
}

func (s *schemaSuite) TestControllerTables(c *gc.C) {
	c.Logf("Committing schema DDL")

	s.applyDDL(c, ControllerDDL())

	// Ensure that each table is present.
	expected := set.NewStrings(
		// Autocert cache
		"autocert_cache",
		"autocert_cache_encoding",

		// Leases
		"lease",
		"lease_type",
		"lease_pin",

		// Change log
		"change_log",
		"change_log_edit_type",
		"change_log_namespace",
		"change_log_witness",

		// Cloud
		"cloud",
		"auth_type",
		"cloud_auth_type",
		"cloud_ca_cert",
		"cloud_credential",
		"cloud_credential_attributes",
		"cloud_defaults",
		"cloud_region",
		"cloud_region_defaults",
		"cloud_type",

		// External controller
		"external_controller",
		"external_controller_address",
		"external_model",

		// Model
		"model_list",
		"model_metadata",
		"model_type",

		// Controller config
		"controller_config",

		// Controller nodes
		"controller_node",

		// Model migration
		"model_migration",
		"model_agent",
		"model_migration_status",
		"model_migration_user",
		"model_migration_minion_sync",

		// Upgrade info
		"upgrade_info",
		"upgrade_info_controller_node",
		"upgrade_state_type",

		// Object store metadata
		"object_store_metadata",
		"object_store_metadata_path",
		"object_store_metadata_hash_type",

		// Users
		"user",
		"user_authentication",
		"user_password",
		"user_activation_key",

		// Flags
		"flag",

		// Permissions
		"permission_access_type",
		"permission_object_access",
		"permission_object_type",
		"permission",

		// Secret backends
		"secret_backend",
		"secret_backend_config",
		"secret_backend_rotation",
		"secret_backend_type",
	)
	c.Assert(readEntityNames(c, s.DB(), "table"), jc.SameContents, expected.Union(internalTableNames).SortedValues())
}

func (s *schemaSuite) TestControllerViews(c *gc.C) {
	c.Logf("Committing schema DDL")

	s.applyDDL(c, ControllerDDL())

	// Ensure that each view is present.
	expected := set.NewStrings(
		// Users
		"v_user_auth",

		// Credentials
		"v_cloud_credential",

		// Models
		"v_model",
	)
	c.Assert(readEntityNames(c, s.DB(), "view"), jc.SameContents, expected.SortedValues())
}

func (s *schemaSuite) TestModelTables(c *gc.C) {
	s.applyDDL(c, ModelDDL())

	// Ensure that each table is present.
	expected := set.NewStrings(
		// Annotations
		"annotation_application",
		"annotation_charm",
		"annotation_machine",
		"annotation_unit",
		"annotation_model",
		"annotation_storage_instance",
		"annotation_storage_filesystem",
		"annotation_storage_volume",

		"life",

		// Change log
		"change_log",
		"change_log_edit_type",
		"change_log_namespace",
		"change_log_witness",

		// Model
		"model",

		// Model config
		"model_config",

		// Object store metadata
		"object_store_metadata",
		"object_store_metadata_path",
		"object_store_metadata_hash_type",

		"application",
		"machine",
		"net_node",
		"cloud_service",
		"cloud_container",
		"unit",

		// Charm
		"charm",
		"charm_storage",

		// Spaces
		"space",
		"provider_space",

		// Subnets
		"subnet",
		"subnet_association_type",
		"subnet_type",
		"subnet_type_association_type",
		"subnet_association",
		"provider_subnet",
		"provider_network",
		"provider_network_subnet",
		"availability_zone",
		"availability_zone_subnet",

		// Block device
		"block_device",
		"filesystem_type",
		"block_device_link_device",

		// Storage
		"storage_pool",
		"storage_pool_attribute",
		"storage_kind",
		"storage_instance",
		"storage_unit_owner",
		"storage_attachment",
		"application_storage_directive",
		"unit_storage_directive",
		"storage_volume",
		"storage_instance_volume",
		"storage_volume_attachment",
		"storage_filesystem",
		"storage_instance_filesystem",
		"storage_filesystem_attachment",
		"storage_volume_attachment_plan",
		"storage_volume_attachment_plan_attr",
		"storage_provisioning_status",
		"storage_volume_device_type",

		// Secret
		"secret_rotate_policy",
		"secret",
		"secret_rotation",
		"secret_value_ref",
		"secret_content",
		"secret_revision",
		"secret_revision_expire",
		"secret_application_owner",
		"secret_model_owner",
		"secret_unit_owner",
		"secret_application_consumer",
		"secret_unit_consumer",
		"secret_remote_application_consumer",
		"secret_remote_unit_consumer",
		"secret_permission",
		"secret_role",
	)
	c.Assert(readEntityNames(c, s.DB(), "table"), jc.SameContents, expected.Union(internalTableNames).SortedValues())
}

func (s *schemaSuite) TestControllerChangeLogTriggers(c *gc.C) {
	s.applyDDL(c, ControllerDDL())

	// Ensure that each trigger is present.
	expected := set.NewStrings(
		"trg_log_cloud_credential_insert",
		"trg_log_cloud_credential_update",
		"trg_log_cloud_credential_delete",

		"trg_log_cloud_insert",
		"trg_log_cloud_update",
		"trg_log_cloud_delete",

		"trg_log_controller_config_insert",
		"trg_log_controller_config_update",
		"trg_log_controller_config_delete",

		"trg_log_controller_node_insert",
		"trg_log_controller_node_update",
		"trg_log_controller_node_delete",

		"trg_log_external_controller_insert",
		"trg_log_external_controller_update",
		"trg_log_external_controller_delete",

		"trg_log_model_migration_minion_sync_insert",
		"trg_log_model_migration_minion_sync_update",
		"trg_log_model_migration_minion_sync_delete",

		"trg_log_model_migration_status_insert",
		"trg_log_model_migration_status_update",
		"trg_log_model_migration_status_delete",

		"trg_log_object_store_metadata_path_insert",
		"trg_log_object_store_metadata_path_update",
		"trg_log_object_store_metadata_path_delete",

		"trg_log_upgrade_info_controller_node_insert",
		"trg_log_upgrade_info_controller_node_update",
		"trg_log_upgrade_info_controller_node_delete",

		"trg_log_upgrade_info_insert",
		"trg_log_upgrade_info_update",
		"trg_log_upgrade_info_delete",

		"trg_log_secret_backend_rotation_next_rotation_time_insert",
		"trg_log_secret_backend_rotation_next_rotation_time_update",
		"trg_log_secret_backend_rotation_next_rotation_time_delete",
	)
	c.Assert(readEntityNames(c, s.DB(), "trigger"), jc.SameContents, expected.SortedValues())
}

func (s *schemaSuite) TestModelChangeLogTriggers(c *gc.C) {
	s.applyDDL(c, ModelDDL())

	// Ensure that each trigger is present.
	expected := set.NewStrings(
		"trg_log_model_config_insert",
		"trg_log_model_config_update",
		"trg_log_model_config_delete",

		"trg_log_object_store_metadata_path_insert",
		"trg_log_object_store_metadata_path_update",
		"trg_log_object_store_metadata_path_delete",

		"trg_log_secret_application_consumer_current_revision_insert",
		"trg_log_secret_application_consumer_current_revision_update",
		"trg_log_secret_application_consumer_current_revision_delete",

		"trg_log_secret_auto_prune_insert",
		"trg_log_secret_auto_prune_update",
		"trg_log_secret_auto_prune_delete",

		"trg_log_secret_remote_application_consumer_current_revision_insert",
		"trg_log_secret_remote_application_consumer_current_revision_update",
		"trg_log_secret_remote_application_consumer_current_revision_delete",

		"trg_log_secret_remote_unit_consumer_current_revision_insert",
		"trg_log_secret_remote_unit_consumer_current_revision_update",
		"trg_log_secret_remote_unit_consumer_current_revision_delete",

		"trg_log_secret_revision_expire_next_expire_time_insert",
		"trg_log_secret_revision_expire_next_expire_time_update",
		"trg_log_secret_revision_expire_next_expire_time_delete",

		"trg_log_secret_revision_obsolete_insert",
		"trg_log_secret_revision_obsolete_update",
		"trg_log_secret_revision_obsolete_delete",

		"trg_log_secret_rotation_next_rotation_time_insert",
		"trg_log_secret_rotation_next_rotation_time_update",
		"trg_log_secret_rotation_next_rotation_time_delete",

		"trg_log_secret_unit_consumer_current_revision_insert",
		"trg_log_secret_unit_consumer_current_revision_update",
		"trg_log_secret_unit_consumer_current_revision_delete",

		"trg_log_block_device_insert",
		"trg_log_block_device_update",
		"trg_log_block_device_delete",

		"trg_log_storage_attachment_insert",
		"trg_log_storage_attachment_update",
		"trg_log_storage_attachment_delete",

		"trg_log_storage_filesystem_attachment_insert",
		"trg_log_storage_filesystem_attachment_update",
		"trg_log_storage_filesystem_attachment_delete",

		"trg_log_storage_filesystem_insert",
		"trg_log_storage_filesystem_update",
		"trg_log_storage_filesystem_delete",

		"trg_log_storage_volume_attachment_insert",
		"trg_log_storage_volume_attachment_update",
		"trg_log_storage_volume_attachment_delete",

		"trg_log_storage_volume_attachment_plan_insert",
		"trg_log_storage_volume_attachment_plan_update",
		"trg_log_storage_volume_attachment_plan_delete",

		"trg_log_storage_volume_insert",
		"trg_log_storage_volume_update",
		"trg_log_storage_volume_delete",
	)

	// These are additional triggers that are not change log triggers, but
	// will be present in the schema.
	additional := set.NewStrings(
		"trg_readonly_model_delete",
		"trg_readonly_model_update",
	)

	c.Assert(readEntityNames(c, s.DB(), "trigger"), jc.SameContents, expected.Union(additional).SortedValues())
}

func (s *schemaSuite) assertChangeLogCount(c *gc.C, editType int, namespaceID tableNamespaceID, expectedCount int) {
	_ = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT COUNT(*) FROM change_log 
WHERE edit_type_id = ? AND namespace_id = ?;`[1:], editType, namespaceID)

		c.Assert(err, jc.ErrorIsNil)
		defer func() { _ = rows.Close() }()

		var count int
		c.Assert(rows.Next(), jc.IsTrue)
		err = rows.Scan(&count)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(count, gc.Equals, expectedCount)
		return nil
	})
}

func (s *schemaSuite) TestControllerChangeLogTriggersForSecretBackends(c *gc.C) {
	s.applyDDL(c, ControllerDDL())

	s.assertChangeLogCount(c, 1, tableSecretBackendRotation, 0)
	s.assertChangeLogCount(c, 2, tableSecretBackendRotation, 0)
	s.assertChangeLogCount(c, 4, tableSecretBackendRotation, 0)

	backendUUID := utils.MustNewUUID().String()

	s.execSQL(c, "INSERT INTO secret_backend (uuid, name, backend_type) VALUES (?, 'myVault', 'vault');", backendUUID)
	s.execSQL(c, "INSERT INTO secret_backend_rotation (backend_uuid, next_rotation_time) VALUES (?, datetime('now', '+1 day'));", backendUUID)
	s.execSQL(c, `UPDATE secret_backend_rotation SET next_rotation_time = datetime('now', '+2 day') WHERE backend_uuid = ?;`, backendUUID)
	s.execSQL(c, `DELETE FROM secret_backend_rotation WHERE backend_uuid = ?;`, backendUUID)

	s.assertChangeLogCount(c, 1, tableSecretBackendRotation, 1)
	s.assertChangeLogCount(c, 2, tableSecretBackendRotation, 1)
	s.assertChangeLogCount(c, 4, tableSecretBackendRotation, 1)
}

func (s *schemaSuite) execSQL(c *gc.C, q string, args ...any) {
	_ = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, args...)
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
}

func (s *schemaSuite) TestModelChangeLogTriggersForSecretTables(c *gc.C) {
	s.applyDDL(c, ModelDDL())

	// secret table triggers.
	s.assertChangeLogCount(c, 1, tableSecretAutoPrune, 0)
	s.assertChangeLogCount(c, 2, tableSecretAutoPrune, 0)
	s.assertChangeLogCount(c, 4, tableSecretAutoPrune, 0)

	secretUUID := utils.MustNewUUID().String()
	s.execSQL(c, `INSERT INTO secret (uuid, description) VALUES (?, 'mySecret');`, secretUUID)
	s.execSQL(c, `UPDATE secret SET auto_prune = true WHERE uuid = ?;`, secretUUID)
	s.execSQL(c, `DELETE FROM secret WHERE uuid = ?;`, secretUUID)

	s.assertChangeLogCount(c, 1, tableSecretAutoPrune, 1)
	s.assertChangeLogCount(c, 2, tableSecretAutoPrune, 1)
	s.assertChangeLogCount(c, 4, tableSecretAutoPrune, 1)

	// secret_rotation table triggers.
	s.assertChangeLogCount(c, 1, tableSecretRotation, 0)
	s.assertChangeLogCount(c, 2, tableSecretRotation, 0)
	s.assertChangeLogCount(c, 4, tableSecretRotation, 0)

	s.execSQL(c, `INSERT INTO secret (uuid, description) VALUES (?, 'mySecret');`, secretUUID)
	s.execSQL(c, `INSERT INTO secret_rotation (secret_uuid, next_rotation_time) VALUES (?, datetime('now', '+1 day'));`, secretUUID)
	s.execSQL(c, `UPDATE secret_rotation SET next_rotation_time = datetime('now', '+2 day') WHERE secret_uuid = ?;`, secretUUID)
	s.execSQL(c, `DELETE FROM secret_rotation WHERE secret_uuid = ?;`, secretUUID)

	s.assertChangeLogCount(c, 1, tableSecretRotation, 1)
	s.assertChangeLogCount(c, 2, tableSecretRotation, 1)
	s.assertChangeLogCount(c, 4, tableSecretRotation, 1)

	// secret_revision table triggers.
	revisionUUID := utils.MustNewUUID().String()
	s.assertChangeLogCount(c, 1, tableSecretRevisionObsolete, 0)
	s.assertChangeLogCount(c, 2, tableSecretRevisionObsolete, 0)
	s.assertChangeLogCount(c, 4, tableSecretRevisionObsolete, 0)

	s.execSQL(c, `INSERT INTO secret_revision (uuid, secret_uuid, revision) VALUES (?, ?, 1);`, revisionUUID, secretUUID)
	s.execSQL(c, `UPDATE secret_revision SET obsolete = true WHERE uuid = ?;`, revisionUUID)
	s.execSQL(c, `DELETE FROM secret_revision WHERE uuid = ?;`, revisionUUID)

	s.assertChangeLogCount(c, 1, tableSecretRevisionObsolete, 1)
	s.assertChangeLogCount(c, 2, tableSecretRevisionObsolete, 1)
	s.assertChangeLogCount(c, 4, tableSecretRevisionObsolete, 1)

	// secret_revision_expire table triggers.
	s.assertChangeLogCount(c, 1, tableSecretRevisionExpire, 0)
	s.assertChangeLogCount(c, 2, tableSecretRevisionExpire, 0)
	s.assertChangeLogCount(c, 4, tableSecretRevisionExpire, 0)

	s.execSQL(c, `INSERT INTO secret_revision (uuid, secret_uuid, revision) VALUES (?, ?, 1);`, revisionUUID, secretUUID)
	s.execSQL(c, `INSERT INTO secret_revision_expire (revision_uuid, next_expire_time) VALUES (?, datetime('now', '+1 day'));`, revisionUUID)
	s.execSQL(c, `UPDATE secret_revision_expire SET next_expire_time = datetime('now', '+2 day') WHERE revision_uuid = ?;`, revisionUUID)
	s.execSQL(c, `DELETE FROM secret_revision_expire WHERE revision_uuid = ?;`, revisionUUID)

	s.assertChangeLogCount(c, 1, tableSecretRevisionExpire, 1)
	s.assertChangeLogCount(c, 2, tableSecretRevisionExpire, 1)
	s.assertChangeLogCount(c, 4, tableSecretRevisionExpire, 1)

	appUUID := utils.MustNewUUID().String()
	s.execSQL(c, `INSERT INTO application (uuid, name, life_id) VALUES (?, 'mysql', 0);`, appUUID)

	netNodeUUID := utils.MustNewUUID().String()
	s.execSQL(c, `INSERT INTO net_node (uuid) VALUES (?);`, netNodeUUID)
	unitUUID := utils.MustNewUUID().String()
	s.execSQL(c, `INSERT INTO unit (uuid, unit_id, application_uuid, net_node_uuid, life_id) VALUES (?, 0, ?, ?, 0);`, unitUUID, appUUID, netNodeUUID)

	// secret_application_consumer table triggers.
	applicationConsumerUUID := utils.MustNewUUID().String()
	s.assertChangeLogCount(c, 1, tableSecretApplicationConsumerCurrentRevision, 0)
	s.assertChangeLogCount(c, 2, tableSecretApplicationConsumerCurrentRevision, 0)
	s.assertChangeLogCount(c, 4, tableSecretApplicationConsumerCurrentRevision, 0)

	s.execSQL(c, `
INSERT INTO secret_application_consumer (uuid, secret_uuid, application_uuid, current_revision) VALUES
	(?, ?, ?, 1);
`, applicationConsumerUUID, secretUUID, appUUID)
	s.execSQL(c, `UPDATE secret_application_consumer SET current_revision = 2 WHERE uuid = ?;`, applicationConsumerUUID)
	s.execSQL(c, `DELETE FROM secret_application_consumer WHERE uuid = ?;`, applicationConsumerUUID)

	s.assertChangeLogCount(c, 1, tableSecretApplicationConsumerCurrentRevision, 1)
	s.assertChangeLogCount(c, 2, tableSecretApplicationConsumerCurrentRevision, 1)
	s.assertChangeLogCount(c, 4, tableSecretApplicationConsumerCurrentRevision, 1)

	// secret_unit_consumer table triggers.
	unitConsumerUUID := utils.MustNewUUID().String()
	s.assertChangeLogCount(c, 1, tableSecretUnitConsumerCurrentRevision, 0)
	s.assertChangeLogCount(c, 2, tableSecretUnitConsumerCurrentRevision, 0)
	s.assertChangeLogCount(c, 4, tableSecretUnitConsumerCurrentRevision, 0)

	s.execSQL(c, `INSERT INTO secret_unit_consumer (uuid, secret_uuid, unit_uuid, current_revision) VALUES (?, ?, ?, 1);`, unitConsumerUUID, secretUUID, unitUUID)
	s.execSQL(c, `UPDATE secret_unit_consumer SET current_revision = 2 WHERE uuid = ?;`, unitConsumerUUID)
	s.execSQL(c, `DELETE FROM secret_unit_consumer WHERE uuid = ?;`, unitConsumerUUID)

	s.assertChangeLogCount(c, 1, tableSecretUnitConsumerCurrentRevision, 1)
	s.assertChangeLogCount(c, 2, tableSecretUnitConsumerCurrentRevision, 1)
	s.assertChangeLogCount(c, 4, tableSecretUnitConsumerCurrentRevision, 1)

	// secret_remote_application_consumer table triggers.
	remoteAppConsumerUUID := utils.MustNewUUID().String()
	s.assertChangeLogCount(c, 1, tableSecretRemoteApplicationConsumerCurrentRevision, 0)
	s.assertChangeLogCount(c, 2, tableSecretRemoteApplicationConsumerCurrentRevision, 0)
	s.assertChangeLogCount(c, 4, tableSecretRemoteApplicationConsumerCurrentRevision, 0)

	s.execSQL(c, `INSERT INTO secret_remote_application_consumer (uuid, secret_uuid, application_uuid, current_revision) VALUES (?, ?, ?, 1);`, remoteAppConsumerUUID, secretUUID, appUUID)
	s.execSQL(c, `UPDATE secret_remote_application_consumer SET current_revision = 2 WHERE uuid = ?;`, remoteAppConsumerUUID)
	s.execSQL(c, `DELETE FROM secret_remote_application_consumer WHERE uuid = ?;`, remoteAppConsumerUUID)

	s.assertChangeLogCount(c, 1, tableSecretRemoteApplicationConsumerCurrentRevision, 1)
	s.assertChangeLogCount(c, 2, tableSecretRemoteApplicationConsumerCurrentRevision, 1)
	s.assertChangeLogCount(c, 4, tableSecretRemoteApplicationConsumerCurrentRevision, 1)

	// secret_remote_unit_consumer table triggers.
	remoteUnitConsumerUUID := utils.MustNewUUID().String()
	s.assertChangeLogCount(c, 1, tableSecretRemoteUnitConsumerCurrentRevision, 0)
	s.assertChangeLogCount(c, 2, tableSecretRemoteUnitConsumerCurrentRevision, 0)
	s.assertChangeLogCount(c, 4, tableSecretRemoteUnitConsumerCurrentRevision, 0)

	s.execSQL(c, `INSERT INTO secret_remote_unit_consumer (uuid, secret_uuid, unit_uuid, current_revision) VALUES(?, ?, ?, 1);`, remoteUnitConsumerUUID, secretUUID, unitUUID)
	s.execSQL(c, `UPDATE secret_remote_unit_consumer SET current_revision = 2 WHERE uuid = ?;`, remoteUnitConsumerUUID)
	s.execSQL(c, `DELETE FROM secret_remote_unit_consumer WHERE uuid = ?;`, remoteUnitConsumerUUID)

	s.assertChangeLogCount(c, 1, tableSecretRemoteUnitConsumerCurrentRevision, 1)
	s.assertChangeLogCount(c, 2, tableSecretRemoteUnitConsumerCurrentRevision, 1)
	s.assertChangeLogCount(c, 4, tableSecretRemoteUnitConsumerCurrentRevision, 1)
}
