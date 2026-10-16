// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"slices"
	"strings"
)

// ColumnStorageType reports the SQL type needed to restore domain bindings.
// Some legacy TEXT columns are bound as bytes. Their declared affinity does
// not convert BLOB bindings to TEXT, and byte-bound lookups need them to remain
// BLOBs after replay. Keep that compatibility knowledge alongside the schema.
// Other columns retain their declared type, normalised to upper case.
func ColumnStorageType(table, column, declaredType string) string {
	if slices.Contains(binaryTextColumns[table], column) {
		return "BLOB"
	}
	return strings.ToUpper(declaredType)
}

// binaryTextColumns records historical storage contracts not expressed by the
// column's declared type. Changing these requires checking domain readers and
// writers as well as existing persisted data.
var binaryTextColumns = map[string][]string{
	"bakery_config": {
		"local_users_private_key", "local_users_public_key",
		"local_users_third_party_private_key", "local_users_third_party_public_key",
		"external_users_third_party_private_key", "external_users_third_party_public_key",
		"offers_third_party_private_key", "offers_third_party_public_key",
	},
	"macaroon_root_key":                            {"id", "root_key"},
	"user_password":                                {"password_salt"},
	"user_activation_key":                          {"activation_key"},
	"charm":                                        {"lxd_profile"},
	"charm_metadata":                               {"assumes"},
	"charm_action":                                 {"params"},
	"machine_status":                               {"data"},
	"machine_cloud_instance_status":                {"data"},
	"operator_status":                              {"data"},
	"application_status":                           {"data"},
	"unit_agent_status":                            {"data"},
	"unit_workload_status":                         {"data"},
	"k8s_pod_status":                               {"data"},
	"application_remote_offerer":                   {"macaroon"},
	"application_remote_offerer_status":            {"data"},
	"application_remote_offerer_relation_macaroon": {"macaroon"},
}
