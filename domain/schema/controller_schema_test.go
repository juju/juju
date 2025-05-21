// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	_ "github.com/mattn/go-sqlite3"
)

type controllerSchemaSuite struct {
	schemaBaseSuite
}

func TestControllerSchemaSuite(t *testing.T) {
	tc.Run(t, &controllerSchemaSuite{})
}

func (s *controllerSchemaSuite) TestControllerTables(c *tc.C) {
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
		"cloud_credential_attribute",
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
		"controller_node_agent_version",

		// Controller API addresses
		"controller_api_address",

		// Model migration
		"model_migration",
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
		"object_store_drain_info",
		"object_store_drain_phase_type",

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

		// Agent binary metadata.
		"agent_binary_store",
	)
	got := readEntityNames(c, s.DB(), "table")
	wanted := expected.Union(internalTableNames)
	c.Assert(got, tc.SameContents, wanted.SortedValues(), tc.Commentf(
		"additive: %v, deletion: %v",
		set.NewStrings(got...).Difference(wanted).SortedValues(),
		wanted.Difference(set.NewStrings(got...)).SortedValues(),
	))
}

func (s *controllerSchemaSuite) TestControllerViews(c *tc.C) {
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
		"v_cloud_credential_attribute",

		// Models
		"v_model",
		"v_model_all",
		"v_model_state",
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

		// Agent binary store
		"v_agent_binary_store",
	)
	c.Assert(readEntityNames(c, s.DB(), "view"), tc.SameContents, expected.SortedValues())
}

func (s *controllerSchemaSuite) TestControllerTriggers(c *tc.C) {
	s.applyDDL(c, ControllerDDL())

	// Expected changelog triggers. Additional triggers are not included and
	// can be added to the addition list.
	expected := set.NewStrings(
		"trg_log_cloud_credential_insert",
		"trg_log_cloud_credential_update",
		"trg_log_cloud_credential_delete",

		"trg_log_cloud_credential_attribute_insert",
		"trg_log_cloud_credential_attribute_update",
		"trg_log_cloud_credential_attribute_delete",

		"trg_log_cloud_insert",
		"trg_log_cloud_update",
		"trg_log_cloud_delete",

		"trg_log_cloud_ca_cert_insert",
		"trg_log_cloud_ca_cert_update",
		"trg_log_cloud_ca_cert_delete",

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

		"trg_log_object_store_drain_info_insert",
		"trg_log_object_store_drain_info_update",
		"trg_log_object_store_drain_info_delete",
	)

	// These are additional triggers that are not change log triggers, but
	// will be present in the schema.
	additional := set.NewStrings(
		"trg_secret_backend_immutable_update",
		"trg_secret_backend_immutable_delete",
	)
	got := readEntityNames(c, s.DB(), "trigger")
	wanted := expected.Union(additional)
	c.Assert(got, tc.SameContents, wanted.SortedValues(), tc.Commentf(
		"additive: %v, deletion: %v",
		set.NewStrings(got...).Difference(wanted).SortedValues(),
		wanted.Difference(set.NewStrings(got...)).SortedValues(),
	))
}

func (s *controllerSchemaSuite) TestControllerTriggersForImmutableTables(c *tc.C) {
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
