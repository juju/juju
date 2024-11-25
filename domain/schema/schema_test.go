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
	coresecrets "github.com/juju/juju/core/secrets"
	jujuversion "github.com/juju/juju/core/version"
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
	if s.Verbose {
		ddl.Hook(func(i int, statement string) error {
			c.Logf("-- Applying schema change %d\n%s\n", i, statement)
			return nil
		})
	}
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
		// Namespaces for DQlite
		"namespace_list",

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
		"model",
		"model_namespace",
		"model_type",

		// Life
		"life",

		// Controller config
		"controller",
		"controller_config",

		// Controller nodes
		"controller_node",

		// Model migration
		"model_migration",
		"model_agent",
		"model_migration_status",
		"model_migration_user",
		"model_migration_minion_sync",
		"model_authorized_keys",

		// Upgrade info
		"upgrade_info",
		"upgrade_info_controller_node",
		"upgrade_state_type",

		// Object store metadata
		"object_store_metadata",
		"object_store_metadata_path",
		"object_store_metadata_hash_type",

		// SSH Keys
		"ssh_fingerprint_hash_algorithm",

		// Users
		"user",
		"user_authentication",
		"user_password",
		"user_activation_key",
		"model_last_login",
		"user_public_ssh_key",

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
		"secret_backend_reference",
		"model_secret_backend",

		// macaroon bakery
		"bakery_config",
		"macaroon_root_key",

		// cloud image metadata
		"architecture",
		"cloud_image_metadata",
	)
	got := readEntityNames(c, s.DB(), "table")
	wanted := expected.Union(internalTableNames)
	c.Assert(got, jc.SameContents, wanted.SortedValues(), gc.Commentf(
		"additive: %v, deletion: %v",
		set.NewStrings(got...).Difference(wanted).SortedValues(),
		wanted.Difference(set.NewStrings(got...)).SortedValues(),
	))
}

func (s *schemaSuite) TestControllerViews(c *gc.C) {
	c.Logf("Committing schema DDL")

	s.applyDDL(c, ControllerDDL())

	// Ensure that each view is present.
	expected := set.NewStrings(
		"v_user_auth",
		"v_user_last_login",

		// Controller and controller config
		"v_controller_config",

		// Cloud
		"v_cloud",
		"v_cloud_auth",

		// v_cloud_credential
		"v_cloud_credential",
		"v_cloud_credential_attributes",

		// Models
		"v_model",
		"v_model_authorized_keys",

		// Secret backends
		"v_model_secret_backend",

		// Permissions
		"v_permission",
		"v_permission_cloud",
		"v_permission_controller",
		"v_permission_model",
		"v_permission_offer",
		"v_everyone_external",

		// Object store metadata
		"v_object_store_metadata",
	)
	c.Assert(readEntityNames(c, s.DB(), "view"), jc.SameContents, expected.SortedValues())
}

func (s *schemaSuite) TestModelTables(c *gc.C) {
	s.applyDDL(c, ModelDDL())

	// Ensure that each table is present.
	expected := set.NewStrings(
		// Application
		"application",
		"application_channel",
		"application_config",
		"application_constraint",
		"application_endpoint_space",
		"application_endpoint_cidr",
		"application_platform",
		"application_setting",
		"application_scale",
		"cloud_service",

		// Annotations
		"annotation_application",
		"annotation_charm",
		"annotation_machine",
		"annotation_unit",
		"annotation_model",
		"annotation_storage_instance",
		"annotation_storage_filesystem",
		"annotation_storage_volume",

		// Block commands
		"block_command",
		"block_command_type",

		// Life
		"life",

		// Password
		"password_hash_algorithm",

		// Change log
		"change_log",
		"change_log_edit_type",
		"change_log_namespace",
		"change_log_witness",

		// Model
		"model",

		// Model config
		"model_config",

		// Model status
		"model_status",
		"model_status_value",

		// Object store metadata
		"object_store_metadata",
		"object_store_metadata_path",
		"object_store_metadata_hash_type",

		// Node
		"net_node",
		"instance_tag",
		"fqdn_address",
		"net_node_fqdn_address",
		"hostname_address",
		"net_node_hostname_address",
		"network_address_scope",

		// Link layer device
		"link_layer_device",
		"link_layer_device_type",
		"virtual_port_type",

		// Network address
		"net_node_ip_address",
		"ip_address_scope",
		"ip_address",
		"ip_address_type",
		"ip_address_origin",
		"ip_address_config_type",
		"ip_address_provider",
		"ip_address_subnet",
		"ip_address_gateway",
		"ip_address_dns_search_domain",
		"ip_address_dns_server_address",

		// Unit
		"unit",
		"unit_resolve_kind",
		"unit_state_charm",
		"unit_state_relation",
		"unit_state",
		"unit_agent",
		"unit_principal",
		"cloud_container",
		"cloud_container_port",
		"unit_agent_status",
		"unit_agent_status_data",
		"unit_workload_status",
		"unit_workload_status_data",
		"cloud_container_status",
		"cloud_container_status_data",
		"unit_agent_status_value",
		"unit_workload_status_value",
		"cloud_container_status_value",

		// Constraint
		"constraint",
		"constraint_tag",
		"constraint_space",
		"constraint_zone",

		// Machine
		"machine",
		"machine_parent",
		"machine_constraint",
		"machine_agent",
		"machine_volume",
		"machine_filesystem",
		"machine_requires_reboot",
		"machine_removals",
		"machine_status",
		"machine_status_data",
		"machine_status_value",
		"machine_cloud_instance",
		"machine_cloud_instance_status_value",
		"machine_cloud_instance_status",
		"machine_cloud_instance_status_data",
		"machine_lxd_profile",

		// Charm
		"architecture",
		"charm_action",
		"charm_category",
		"charm_config_type",
		"charm_config",
		"charm_container_mount",
		"charm_container",
		"charm_device",
		"charm_extra_binding",
		"charm_hash",
		"charm_manifest_base",
		"charm_metadata",
		"charm_origin",
		"charm_payload",
		"charm_platform",
		"charm_relation_kind",
		"charm_relation_role",
		"charm_relation_scope",
		"charm_relation",
		"charm_resource_kind",
		"charm_resource",
		"charm_run_as_kind",
		"charm_source",
		"charm_storage_kind",
		"charm_storage_property",
		"charm_storage",
		"charm_tag",
		"charm_term",
		"charm",
		"hash_kind",
		"os",

		// Resources
		"application_resource",
		"kubernetes_application_resource",
		"resource",
		"resource_container_image_metadata_store",
		"resource_file_store",
		"resource_image_store",
		"resource_origin_type",
		"resource_retrieved_by",
		"resource_retrieved_by_type",
		"resource_state",
		"unit_resource",

		// Space
		"space",
		"provider_space",

		// Subnet
		"subnet",
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
		"secret_reference",
		"secret_metadata",
		"secret_rotation",
		"secret_value_ref",
		"secret_deleted_value_ref",
		"secret_content",
		"secret_revision",
		"secret_revision_obsolete",
		"secret_revision_expire",
		"secret_application_owner",
		"secret_model_owner",
		"secret_unit_owner",
		"secret_unit_consumer",
		"secret_remote_unit_consumer",
		"secret_permission",
		"secret_role",
		"secret_grant_subject_type",
		"secret_grant_scope_type",

		// Opened Ports
		"protocol",
		"port_range",
	)
	got := readEntityNames(c, s.DB(), "table")
	wanted := expected.Union(internalTableNames)
	c.Assert(got, jc.SameContents, wanted.SortedValues(), gc.Commentf(
		"additive: %v, deletion: %v",
		set.NewStrings(got...).Difference(wanted).SortedValues(),
		wanted.Difference(set.NewStrings(got...)).SortedValues(),
	))
}

func (s *schemaSuite) TestModelViews(c *gc.C) {
	c.Logf("Committing schema DDL")

	s.applyDDL(c, ModelDDL())

	// Ensure that each view is present.
	expected := set.NewStrings(
		"v_address",
		"v_charm_annotation_index",
		"v_charm_config",
		"v_charm_container",
		"v_charm_list_name_origin",
		"v_charm_manifest",
		"v_charm_metadata",
		"v_charm_origin",
		"v_charm_platform",
		"v_charm_relation",
		"v_charm_resource",
		"v_charm_storage",
		"v_hardware_characteristics",
		"v_port_range",
		"v_endpoint",
		"v_secret_permission",
		"v_space_subnet",

		// Object store metadata
		"v_object_store_metadata",
	)
	c.Assert(readEntityNames(c, s.DB(), "view"), jc.SameContents, expected.SortedValues())
}

func (s *schemaSuite) TestControllerTriggers(c *gc.C) {
	s.applyDDL(c, ControllerDDL())

	// Expected changelog triggers. Additional triggers are not included and
	// can be added to the addition list.
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

		"trg_log_secret_backend_rotation_insert",
		"trg_log_secret_backend_rotation_update",
		"trg_log_secret_backend_rotation_delete",

		"trg_log_model_secret_backend_insert",
		"trg_log_model_secret_backend_update",
		"trg_log_model_secret_backend_delete",

		"trg_log_model_authorized_keys_insert",
		"trg_log_model_authorized_keys_update",
		"trg_log_model_authorized_keys_delete",

		"trg_log_model_insert",
		"trg_log_model_update",
		"trg_log_model_delete",

		"trg_log_user_authentication_insert",
		"trg_log_user_authentication_update",
		"trg_log_user_authentication_delete",

		"trg_log_model_agent_insert",
		"trg_log_model_agent_update",
		"trg_log_model_agent_delete",
	)

	// These are additional triggers that are not change log triggers, but
	// will be present in the schema.
	additional := set.NewStrings(
		"trg_secret_backend_immutable_update",
		"trg_secret_backend_immutable_delete",
	)
	got := readEntityNames(c, s.DB(), "trigger")
	wanted := expected.Union(additional)
	c.Assert(got, jc.SameContents, wanted.SortedValues(), gc.Commentf(
		"additive: %v, deletion: %v",
		set.NewStrings(got...).Difference(wanted).SortedValues(),
		wanted.Difference(set.NewStrings(got...)).SortedValues(),
	))
}

func (s *schemaSuite) TestModelTriggers(c *gc.C) {
	s.applyDDL(c, ModelDDL())

	// Expected changelog triggers. Additional triggers are not included and
	// can be added to the addition list.
	expected := set.NewStrings(
		"trg_log_application_delete",
		"trg_log_application_insert",
		"trg_log_application_update",

		"trg_log_application_scale_delete",
		"trg_log_application_scale_insert",
		"trg_log_application_scale_update",

		"trg_log_block_device_delete",
		"trg_log_block_device_insert",
		"trg_log_block_device_update",

		"trg_log_charm_delete",
		"trg_log_charm_insert",
		"trg_log_charm_update",

		"trg_log_machine_cloud_instance_delete",
		"trg_log_machine_cloud_instance_insert",
		"trg_log_machine_cloud_instance_update",

		"trg_log_machine_delete",
		"trg_log_machine_insert",
		"trg_log_machine_update",

		"trg_log_machine_lxd_profile_delete",
		"trg_log_machine_lxd_profile_insert",
		"trg_log_machine_lxd_profile_update",

		"trg_log_machine_requires_reboot_delete",
		"trg_log_machine_requires_reboot_insert",
		"trg_log_machine_requires_reboot_update",

		"trg_log_model_config_delete",
		"trg_log_model_config_insert",
		"trg_log_model_config_update",

		"trg_log_object_store_metadata_path_delete",
		"trg_log_object_store_metadata_path_insert",
		"trg_log_object_store_metadata_path_update",

		"trg_log_port_range_delete",
		"trg_log_port_range_insert",
		"trg_log_port_range_update",

		"trg_log_secret_deleted_value_ref_delete",
		"trg_log_secret_deleted_value_ref_insert",
		"trg_log_secret_deleted_value_ref_update",

		"trg_log_secret_metadata_delete",
		"trg_log_secret_metadata_insert",
		"trg_log_secret_metadata_update",

		"trg_log_secret_reference_delete",
		"trg_log_secret_reference_insert",
		"trg_log_secret_reference_update",

		"trg_log_secret_revision_delete",
		"trg_log_secret_revision_insert",
		"trg_log_secret_revision_update",

		"trg_log_secret_revision_expire_delete",
		"trg_log_secret_revision_expire_insert",
		"trg_log_secret_revision_expire_update",

		"trg_log_secret_revision_obsolete_delete",
		"trg_log_secret_revision_obsolete_insert",
		"trg_log_secret_revision_obsolete_update",

		"trg_log_secret_rotation_delete",
		"trg_log_secret_rotation_insert",
		"trg_log_secret_rotation_update",

		"trg_log_storage_attachment_delete",
		"trg_log_storage_attachment_insert",
		"trg_log_storage_attachment_update",

		"trg_log_storage_filesystem_attachment_delete",
		"trg_log_storage_filesystem_attachment_insert",
		"trg_log_storage_filesystem_attachment_update",

		"trg_log_storage_filesystem_delete",
		"trg_log_storage_filesystem_insert",
		"trg_log_storage_filesystem_update",

		"trg_log_storage_volume_attachment_delete",
		"trg_log_storage_volume_attachment_insert",
		"trg_log_storage_volume_attachment_update",

		"trg_log_storage_volume_attachment_plan_delete",
		"trg_log_storage_volume_attachment_plan_insert",
		"trg_log_storage_volume_attachment_plan_update",

		"trg_log_storage_volume_delete",
		"trg_log_storage_volume_insert",
		"trg_log_storage_volume_update",

		"trg_log_subnet_delete",
		"trg_log_subnet_insert",
		"trg_log_subnet_update",

		"trg_log_unit_delete",
		"trg_log_unit_insert",
		"trg_log_unit_update",
	)

	// These are additional triggers that are not change log triggers, but
	// will be present in the schema.
	additional := set.NewStrings(
		"trg_model_immutable_delete",
		"trg_model_immutable_update",
		"trg_secret_permission_immutable_update",
	)

	got := readEntityNames(c, s.DB(), "trigger")
	wanted := expected.Union(additional)
	c.Assert(got, jc.SameContents, wanted.SortedValues(), gc.Commentf(
		"additive: %v, deletion: %v",
		set.NewStrings(got...).Difference(wanted).SortedValues(),
		wanted.Difference(set.NewStrings(got...)).SortedValues(),
	))
}

func (s *schemaSuite) assertChangeLogCount(c *gc.C, editType int, namespaceID tableNamespaceID, expectedCount int) {
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
			return fmt.Errorf("no rows returned")
		}
		return rows.Scan(&count)
	})
	c.Assert(count, gc.Equals, expectedCount)
}

func (s *schemaSuite) TestControllerChangeLogTriggersForSecretBackends(c *gc.C) {
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

func (s *schemaSuite) assertExecSQL(c *gc.C, q string, args ...any) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *schemaSuite) assertExecSQLError(c *gc.C, q string, errMsg string, args ...any) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	})
	c.Assert(err, gc.ErrorMatches, errMsg)
}

func (s *schemaSuite) TestModelChangeLogTriggersForSecretTables(c *gc.C) {
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
	s.assertExecSQL(c, "INSERT INTO charm (uuid) VALUES (?);", charmUUID)
	s.assertExecSQL(c, "INSERT INTO charm_metadata (charm_uuid, name) VALUES (?, 'mysql');", charmUUID)
	s.assertExecSQL(c, "INSERT INTO charm_origin (charm_uuid, reference_name) VALUES (?, 'mysql');", charmUUID)

	appUUID := utils.MustNewUUID().String()
	s.assertExecSQL(c, `
INSERT INTO application (uuid, charm_uuid, name, life_id, password_hash_algorithm_id, password_hash)
VALUES (?, ?, 'mysql', 0, 0, 'K68fQBBdlQH+MZqOxGP99DJaKl30Ra3z9XL2JiU2eMk=');`, appUUID, charmUUID)

	netNodeUUID := utils.MustNewUUID().String()
	s.assertExecSQL(c, `INSERT INTO net_node (uuid) VALUES (?);`, netNodeUUID)
	unitUUID := utils.MustNewUUID().String()
	s.assertExecSQL(c, `
INSERT INTO unit (uuid, life_id, name, application_uuid, net_node_uuid, charm_uuid, resolve_kind_id)
VALUES (?, 0, 0, ?, ?, ?, 0);`,
		unitUUID, appUUID, netNodeUUID, charmUUID)
}

func (s *schemaSuite) TestControllerTriggersForImmutableTables(c *gc.C) {
	s.applyDDL(c, ControllerDDL())

	backendUUID1 := utils.MustNewUUID().String()
	backendUUID2 := utils.MustNewUUID().String()
	s.assertExecSQL(c,
		"INSERT INTO secret_backend (uuid, name, backend_type_id) VALUES (?, 'controller-sb', 0);",
		backendUUID1)
	s.assertExecSQL(c,
		"INSERT INTO secret_backend (uuid, name, backend_type_id) VALUES (?, 'kubernetes-sb', 1);",
		backendUUID2)
	s.assertExecSQLError(c,
		"UPDATE secret_backend SET name = 'new-name' WHERE uuid = ?",
		"secret backends with type controller or kubernetes are immutable", backendUUID1)
	s.assertExecSQLError(c,
		"UPDATE secret_backend SET name = 'new-name' WHERE uuid = ?",
		"secret backends with type controller or kubernetes are immutable", backendUUID2)

	s.assertExecSQLError(c,
		"DELETE FROM secret_backend WHERE uuid = ?;",
		"secret backends with type controller or kubernetes are immutable", backendUUID1)
	s.assertExecSQLError(c,
		"DELETE FROM secret_backend WHERE uuid = ?;",
		"secret backends with type controller or kubernetes are immutable", backendUUID2)
}

func (s *schemaSuite) TestModelTriggersForImmutableTables(c *gc.C) {
	s.applyDDL(c, ModelDDL())

	modelUUID := utils.MustNewUUID().String()
	controllerUUID := utils.MustNewUUID().String()
	s.assertExecSQL(c, `
INSERT INTO model (uuid, controller_uuid, target_agent_version, name, type, cloud, cloud_type, cloud_region)
VALUES (?, ?, ?, 'my-model', 'caas', 'cloud-1', 'kubernetes', 'cloud-region-1');`,
		modelUUID, controllerUUID, jujuversion.Current.String())
	s.assertExecSQLError(c,
		"UPDATE model SET name = 'new-name' WHERE uuid = ?",
		"model table is immutable", modelUUID)

	s.assertExecSQLError(c,
		"DELETE FROM model WHERE uuid = ?;",
		"model table is immutable", modelUUID)
}
