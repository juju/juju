// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	controllertypes "github.com/juju/juju/domain/export/types/controller/v4_1_0"
	modeltypes "github.com/juju/juju/domain/export/types/v4_1_0"
	coretesting "github.com/juju/juju/internal/testing"
)

type lookupKeyDecodeSuite struct{ coretesting.BaseSuite }

func TestLookupKeyDecodeSuite(t *testing.T) { tc.Run(t, &lookupKeyDecodeSuite{}) }

func (s *lookupKeyDecodeSuite) TestModelLookupKeys(c *tc.C) {
	// The 48 affected model keys, excluding the two target-local change-log
	// lookups that recovery deliberately does not decode or replay.
	tables := strings.Fields(`agent_stream architecture block_command_type
		block_device_provenance charm_config_type charm_provenance
		charm_relation_role charm_relation_scope charm_resource_kind
		charm_run_as_kind charm_source charm_storage_kind container_type hash_kind
		ip_address_config_type ip_address_origin ip_address_scope ip_address_type
		k8s_pod_status_value life link_layer_device_type
		machine_cloud_instance_status_value machine_placement_scope machine_status_value
		network_address_scope operation_task_status_value os password_hash_algorithm
		protocol resolve_mode resource_origin_type resource_retrieved_by_type
		resource_state secret_grant_scope_type secret_grant_subject_type secret_role
		secret_rotate_policy ssh_key_algorithm_type storage_filesystem_status_value
		storage_kind storage_provision_scope storage_volume_device_type
		storage_volume_status_value unit_agent_status_value virtual_port_type
		workload_status_value`)
	c.Assert(tables, tc.HasLen, 46)
	checkLookupDumpKeys(c, reflect.TypeFor[modeltypes.ModelExport](), tables, func(data []byte) (reflect.Value, error) {
		dump, err := DecodeModelDump(data)
		if err != nil {
			c.Check(dump, tc.IsNil)
			return reflect.Value{}, err
		}
		return reflect.ValueOf(dump.ModelExport), nil
	})
}

func (s *lookupKeyDecodeSuite) TestControllerLookupKeys(c *tc.C) {
	tables := strings.Fields(`architecture auth_type cloud_type lease_type life model_type
		object_store_drain_phase_type password_hash_algorithm permission_access_type
		permission_object_access permission_object_type secret_backend_type
		ssh_fingerprint_hash_algorithm ssh_key_algorithm_type upgrade_state_type`)
	c.Assert(tables, tc.HasLen, 15)
	checkLookupDumpKeys(c, reflect.TypeFor[controllertypes.ControllerExport](), tables, func(data []byte) (reflect.Value, error) {
		dump, err := DecodeControllerDump(data)
		if err != nil {
			c.Check(dump, tc.IsNil)
			return reflect.Value{}, err
		}
		return reflect.ValueOf(dump.ControllerExport), nil
	})
}

func (s *lookupKeyDecodeSuite) TestExcludedLookupKeys(c *tc.C) {
	// Malformed source rows must not introduce a new requirement on tables
	// whose schema seeds belong to the target, including their zero IDs.
	data := []byte(`payload:
  change_log_edit_type: [{id: null, type: ignored}, {type: omitted}]
  change_log_namespace: [{id: null, namespace: ignored}, {namespace: omitted}]
`)
	model, err := DecodeModelDump(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(model.Present, tc.HasLen, 0)
	c.Check(model.ChangeLogEditType, tc.HasLen, 0)
	c.Check(model.ChangeLogNamespace, tc.HasLen, 0)
	controller, err := DecodeControllerDump(data)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controller.Present, tc.HasLen, 0)
	c.Check(controller.ChangeLogEditType, tc.HasLen, 0)
	c.Check(controller.ChangeLogNamespace, tc.HasLen, 0)
}

func checkLookupDumpKeys(c *tc.C, exportType reflect.Type, tables []string, decode func([]byte) (reflect.Value, error)) {
	for _, table := range tables {
		var field reflect.StructField
		for candidate := range exportType.Fields() {
			if candidate.Tag.Get("yaml") == table {
				field = candidate
				break
			}
		}
		c.Assert(field.Type, tc.NotNil, tc.Commentf("table %s", table))
		rowType := field.Type.Elem()
		idField, ok := rowType.FieldByName("ID")
		c.Assert(ok, tc.IsTrue, tc.Commentf("table %s", table))
		c.Assert(idField.Type, tc.Equals, reflect.TypeFor[int64](), tc.Commentf("table %s", table))
		rows := reflect.MakeSlice(field.Type, 2, 2)
		for i, id := range []int64{0, 7} {
			rows.Index(i).FieldByName("ID").SetInt(id)
			for column := range rowType.Fields() {
				value := rows.Index(i).FieldByIndex(column.Index)
				if value.Kind() == reflect.String {
					value.SetString(fmt.Sprintf("%s-%d", column.Tag.Get("yaml"), i))
				} else if i == 1 && value.Kind() == reflect.Pointer && value.Type().Elem().Kind() == reflect.String {
					value.Set(reflect.New(value.Type().Elem()))
					value.Elem().SetString(fmt.Sprintf("%s-%d", column.Tag.Get("yaml"), i))
				}
			}
		}
		data, err := yaml.Marshal(map[string]any{"payload": map[string]any{table: rows.Interface()}})
		c.Assert(err, tc.ErrorIsNil)
		decoded, err := decode(data)
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("table %s", table))
		c.Check(decoded.FieldByIndex(field.Index).Interface(), tc.DeepEquals, rows.Interface(), tc.Commentf("table %s", table))

		var raw struct {
			Payload map[string][]map[string]any `yaml:"payload"`
		}
		c.Assert(yaml.Unmarshal(data, &raw), tc.ErrorIsNil)
		for _, missing := range []bool{false, true} {
			if missing {
				delete(raw.Payload[table][1], "id")
			} else {
				raw.Payload[table][1]["id"] = nil
			}
			malformed, err := yaml.Marshal(raw)
			c.Assert(err, tc.ErrorIsNil)
			_, err = decode(malformed)
			message := fmt.Sprintf(`table %q: row 1 column "id" cannot be null`, table)
			if missing {
				message = fmt.Sprintf(`table %q: row 1 is missing column "id"`, table)
			}
			c.Check(err, tc.ErrorMatches, message)
		}
	}
}
