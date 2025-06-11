// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"github.com/juju/utils/v4"
	_ "github.com/mattn/go-sqlite3"

	charmtesting "github.com/juju/juju/core/charm/testing"
)

type modelSchemaSuite struct {
	schemaBaseSuite
}

func TestModelSchemaSuite(t *testing.T) {
	tc.Run(t, &modelSchemaSuite{})
}

func (s *modelSchemaSuite) TestModelTables(c *tc.C) {
	s.applyDDL(c, ModelDDL())

	// Ensure that each table is present.
	expected := set.NewStrings(
		// Application
		"application",
		"application_channel",
		"application_config_hash",
		"application_config",
		"application_constraint",
		"application_exposed_endpoint_cidr",
		"application_exposed_endpoint_space",
		"application_platform",
		"application_scale",
		"application_setting",
		"application_status",
		"application_workload_version",
		"k8s_service",
		"workload_status_value",
		"device_constraint",
		"device_constraint_attribute",

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
		"agent_stream",
		"agent_version",

		// Model config
		"model_config",
		"model_constraint",

		// Object store metadata
		"object_store_metadata",
		"object_store_metadata_path",

		// Node
		"fqdn_address",
		"hostname_address",
		"instance_tag",
		"net_node_fqdn_address",
		"net_node_hostname_address",
		"net_node",
		"network_address_scope",

		// Link layer device
		"link_layer_device",
		"link_layer_device_dns_domain",
		"link_layer_device_dns_address",
		"link_layer_device_parent",
		"link_layer_device_route",
		"link_layer_device_type",
		"provider_link_layer_device",
		"virtual_port_type",

		// Network address
		"ip_address_scope",
		"ip_address",
		"ip_address_type",
		"ip_address_origin",
		"ip_address_config_type",
		"provider_ip_address",

		// Unit
		"k8s_pod_port",
		"k8s_pod_status_value",
		"k8s_pod_status",
		"k8s_pod",
		"unit_agent_status_value",
		"unit_agent_status",
		"unit_agent_presence",
		"unit_agent_version",
		"unit_constraint",
		"unit_principal",
		"unit_state_charm",
		"unit_state_relation",
		"unit_state",
		"unit_workload_status",
		"unit_workload_version",
		"unit",

		// Resolve
		"unit_resolved",
		"resolve_mode",

		// Constraint
		"constraint",
		"constraint_tag",
		"constraint_space",
		"constraint_zone",

		// Machine
		"container_type",
		"machine",
		"machine_agent_presence",
		"machine_agent_version",
		"machine_cloud_instance_status_value",
		"machine_cloud_instance_status",
		"machine_cloud_instance",
		"machine_constraint",
		"machine_filesystem",
		"machine_lxd_profile",
		"machine_parent",
		"machine_placement_scope",
		"machine_platform",
		"machine_placement",
		"machine_removals",
		"machine_requires_reboot",
		"machine_status_value",
		"machine_status",
		"machine_volume",

		// Charm
		"architecture",
		"charm_action",
		"charm_category",
		"charm_config_type",
		"charm_config",
		"charm_container_mount",
		"charm_container",
		"charm_device",
		"charm_download_info",
		"charm_extra_binding",
		"charm_hash",
		"charm_manifest_base",
		"charm_metadata",
		"charm_provenance",
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
		"pending_application_resource",
		"resource_container_image_metadata_store",
		"resource_file_store",
		"resource_image_store",
		"resource_origin_type",
		"resource_retrieved_by_type",
		"resource_retrieved_by",
		"resource_state",
		"resource",
		"unit_resource",

		// Space
		"provider_space",
		"space",

		// Subnet
		"availability_zone_subnet",
		"availability_zone",
		"provider_network_subnet",
		"provider_network",
		"provider_subnet",
		"subnet",

		// Block device
		"block_device_link_device",
		"block_device",
		"filesystem_type",

		// Storage
		"application_storage_directive",
		"storage_attachment",
		"storage_filesystem_attachment",
		"storage_filesystem",
		"storage_filesystem_status",
		"storage_filesystem_status_value",
		"storage_instance_filesystem",
		"storage_instance_volume",
		"storage_instance",
		"storage_pool_attribute",
		"storage_pool",
		"storage_unit_owner",
		"storage_volume_attachment_plan_attr",
		"storage_volume_attachment_plan",
		"storage_volume_attachment",
		"storage_volume_device_type",
		"storage_volume",
		"storage_volume_status",
		"storage_volume_status_value",
		"unit_storage_directive",

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

		// Relations
		"application_endpoint",
		"application_extra_endpoint",
		"relation_application_setting",
		"relation_application_settings_hash",
		"relation_endpoint",
		"relation_status_type",
		"relation_status",
		"relation_unit_setting",
		"relation_unit_settings_hash",
		"relation_unit",
		"relation",

		// Cleanup
		"removal_type",
		"removal",

		// Sequence
		"sequence",

		// Agent binary store.
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

func (s *modelSchemaSuite) TestModelViews(c *tc.C) {
	c.Logf("Committing schema DDL")

	s.applyDDL(c, ModelDDL())

	// Ensure that each view is present.
	expected := set.NewStrings(
		"v_address",
		"v_application_charm_download_info",
		"v_application_config",
		"v_application_constraint",
		"v_application_endpoint",
		"v_application_endpoint_uuid",
		"v_application_export",
		"v_application_exposed_endpoint",
		"v_application_origin",
		"v_application_platform_channel",
		"v_application_resource",
		"v_application_storage_directive",
		"v_application_subordinate",
		"v_charm_annotation_index",
		"v_charm_config",
		"v_charm_container",
		"v_charm_manifest",
		"v_charm_metadata",
		"v_charm_relation",
		"v_charm_resource",
		"v_charm_storage",
		"v_constraint",
		"v_endpoint",
		"v_hardware_characteristics",
		"v_machine_agent_version",
		"v_machine_cloud_instance_status",
		"v_machine_interface",
		"v_machine_status",
		"v_machine_target_agent_version",
		"v_model_constraint_space",
		"v_model_constraint_tag",
		"v_model_constraint_zone",
		"v_model_constraint",
		"v_model_metrics",
		"v_object_store_metadata",
		"v_port_range",
		"v_relation_endpoint",
		"v_relation_endpoint_identifier",
		"v_relation_status",
		"v_resource",
		"v_revision_updater_application_unit",
		"v_revision_updater_application",
		"v_secret_permission",
		"v_space_subnet",
		"v_storage_instance",
		"v_unit_agent_presence",
		"v_unit_agent_status",
		"v_unit_attribute",
		"v_unit_constraint",
		"v_unit_password_hash",
		"v_unit_export",
		"v_unit_resource",
		"v_unit_storage_directive",
		"v_unit_target_agent_version",
		"v_unit_workload_status",
		"v_unit_workload_agent_status",
		"v_unit_k8s_pod_status",
		"v_full_unit_status",
		"v_agent_binary_store",
	)
	got := readEntityNames(c, s.DB(), "view")
	c.Assert(got, tc.SameContents, expected.SortedValues(), tc.Commentf(
		"additive: %v, deletion: %v",
		set.NewStrings(got...).Difference(expected).SortedValues(),
		expected.Difference(set.NewStrings(got...)).SortedValues(),
	))
}

func (s *modelSchemaSuite) TestModelTriggers(c *tc.C) {
	s.applyDDL(c, ModelDDL())

	// Expected changelog triggers. Additional triggers are not included and
	// can be added to the addition list.
	expected := set.NewStrings(
		"trg_log_agent_version_update",

		"trg_log_application_delete",
		"trg_log_application_insert",
		"trg_log_application_update",

		"trg_log_application_config_hash_delete",
		"trg_log_application_config_hash_insert",
		"trg_log_application_config_hash_update",

		"trg_log_application_setting_delete",
		"trg_log_application_setting_insert",
		"trg_log_application_setting_update",

		"trg_log_application_endpoint_delete",
		"trg_log_application_endpoint_insert",
		"trg_log_application_endpoint_update",

		"trg_log_application_exposed_endpoint_cidr_delete",
		"trg_log_application_exposed_endpoint_cidr_insert",
		"trg_log_application_exposed_endpoint_cidr_update",

		"trg_log_application_exposed_endpoint_space_delete",
		"trg_log_application_exposed_endpoint_space_insert",
		"trg_log_application_exposed_endpoint_space_update",

		"trg_log_application_scale_delete",
		"trg_log_application_scale_insert",
		"trg_log_application_scale_update",

		"trg_log_block_device_delete",
		"trg_log_block_device_insert",
		"trg_log_block_device_update",

		"trg_log_charm_delete",
		"trg_log_charm_insert",
		"trg_log_charm_update",

		"trg_log_ip_address_delete",
		"trg_log_ip_address_insert",
		"trg_log_ip_address_update",

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

		"trg_log_relation_application_settings_hash_delete",
		"trg_log_relation_application_settings_hash_insert",
		"trg_log_relation_application_settings_hash_update",

		"trg_log_relation_unit_settings_hash_delete",
		"trg_log_relation_unit_settings_hash_insert",
		"trg_log_relation_unit_settings_hash_update",

		"trg_log_relation_delete",
		"trg_log_relation_insert",
		"trg_log_relation_update",

		"trg_log_relation_status_delete",
		"trg_log_relation_status_insert",
		"trg_log_relation_status_update",

		"trg_log_relation_unit_delete",
		"trg_log_relation_unit_insert",
		"trg_log_relation_unit_update",

		"trg_log_unit_delete",
		"trg_log_unit_insert",
		"trg_log_unit_update",

		"trg_log_unit_principal_delete",
		"trg_log_unit_principal_insert",
		"trg_log_unit_principal_update",

		"trg_log_unit_resolved_delete",
		"trg_log_unit_resolved_insert",
		"trg_log_unit_resolved_update",

		"trg_log_removal_delete",
		"trg_log_removal_insert",
		"trg_log_removal_update",

		"trg_log_unit_insert_delete_insert",
		"trg_log_unit_insert_delete_delete",
	)

	// These are additional triggers that are not change log triggers, but
	// will be present in the schema.
	additional := set.NewStrings(
		"trg_model_immutable_delete",
		"trg_model_immutable_update",

		"trg_secret_permission_guard_update",
		"trg_sequence_guard_update",

		"trg_charm_action_immutable_update",
		"trg_charm_config_immutable_update",
		"trg_charm_container_immutable_update",
		"trg_charm_container_mount_immutable_update",
		"trg_charm_device_immutable_update",
		"trg_charm_extra_binding_immutable_update",
		"trg_charm_hash_immutable_update",
		"trg_charm_manifest_base_immutable_update",
		"trg_charm_metadata_immutable_update",
		"trg_charm_relation_immutable_update",
		"trg_charm_resource_immutable_update",
		"trg_charm_storage_immutable_update",
		"trg_charm_term_immutable_update",
	)

	got := readEntityNames(c, s.DB(), "trigger")
	wanted := expected.Union(additional)
	c.Assert(got, tc.SameContents, wanted.SortedValues(), tc.Commentf(
		"additive: %v, deletion: %v",
		set.NewStrings(got...).Difference(wanted).SortedValues(),
		wanted.Difference(set.NewStrings(got...)).SortedValues(),
	))
}

func (s *modelSchemaSuite) TestModelTriggersForImmutableTables(c *tc.C) {
	s.applyDDL(c, ModelDDL())

	modelUUID := utils.MustNewUUID().String()
	controllerUUID := utils.MustNewUUID().String()
	s.assertExecSQL(c, `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type, cloud_region)
VALUES (?, ?, 'my-model', 'prod', 'caas', 'cloud-1', 'kubernetes', 'cloud-region-1');`,
		modelUUID, controllerUUID)

	s.assertExecSQLError(c,
		"UPDATE model SET name = 'new-name' WHERE uuid = ?",
		"model table is immutable, only insertions are allowed", modelUUID)

	s.assertExecSQLError(c,
		"DELETE FROM model WHERE uuid = ?;",
		"model table is immutable, only insertions are allowed", modelUUID)
}

func (s *modelSchemaSuite) TestTriggersForUnmodifiableTables(c *tc.C) {
	s.applyDDL(c, ModelDDL())

	id := charmtesting.GenCharmID(c)

	s.assertExecSQL(c, `
INSERT INTO charm (uuid, reference_name, architecture_id, revision)
VALUES (?, 'foo', 0, 1)
`, id.String())
	s.assertExecSQL(c, `
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, 'foo');`,
		id)
	s.assertExecSQLError(c,
		"UPDATE charm_metadata SET name = 'new-name' WHERE charm_uuid = ?",
		"charm_metadata table is unmodifiable, only insertions and deletions are allowed", id)

	s.assertExecSQL(c, "DELETE FROM charm_metadata WHERE charm_uuid = ?;", id)
}
