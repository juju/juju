// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package export

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/juju/tc"
	"gopkg.in/yaml.v3"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	ctrlv4_1_0 "github.com/juju/juju/domain/export/types/controller/v4_1_0"
	v4_0_12 "github.com/juju/juju/domain/export/types/v4_0_12"
	v4_1_0 "github.com/juju/juju/domain/export/types/v4_1_0"
	coretesting "github.com/juju/juju/internal/testing"
)

type lookupPayloadSuite struct {
	coretesting.BaseSuite
}

func TestLookupPayloadSuite(t *testing.T) {
	tc.Run(t, &lookupPayloadSuite{})
}

func (s *lookupPayloadSuite) TestModelLookupIDs(c *tc.C) {
	// Inventory at 8369e6e2a9, excluding the two target-local changelog tables.
	tables := strings.Fields(`
agent_stream architecture block_command_type block_device_provenance
charm_config_type charm_provenance charm_relation_role charm_relation_scope
charm_resource_kind charm_run_as_kind charm_source charm_storage_kind
container_type hash_kind ip_address_config_type ip_address_origin
ip_address_scope ip_address_type k8s_pod_status_value life link_layer_device_type
machine_cloud_instance_status_value machine_placement_scope machine_status_value
network_address_scope operation_task_status_value os password_hash_algorithm
protocol resolve_mode resource_origin_type resource_retrieved_by_type
resource_state secret_grant_scope_type secret_grant_subject_type secret_role
secret_rotate_policy ssh_key_algorithm_type storage_filesystem_status_value
storage_kind storage_provision_scope storage_volume_device_type
storage_volume_status_value unit_agent_status_value virtual_port_type
workload_status_value
`)
	c.Check(len(tables), tc.Equals, 46)
	s.checkLookupIDs(c, DecodePayload, tables)
}

func (s *lookupPayloadSuite) TestControllerLookupIDs(c *tc.C) {
	tables := strings.Fields(`
architecture auth_type change_log_edit_type change_log_namespace cloud_type
lease_type life model_type object_store_drain_phase_type password_hash_algorithm
permission_access_type permission_object_access permission_object_type
secret_backend_type ssh_fingerprint_hash_algorithm ssh_key_algorithm_type
upgrade_state_type
`)
	c.Check(len(tables), tc.Equals, 17)
	s.checkLookupIDs(c, DecodeControllerPayload, tables)
}

func (s *lookupPayloadSuite) checkLookupIDs(
	c *tc.C, decode func(semversion.Number, []byte) (any, error), tables []string,
) {
	version := semversion.MustParse("4.1.0")
	for _, table := range tables {
		for _, id := range []int64{0, 42} {
			data := []byte(fmt.Sprintf("%s:\n- {id: %d}\n", table, id))
			payload, err := decode(version, data)
			c.Assert(err, tc.ErrorIsNil, tc.Commentf("%s id %d", table, id))
			value := reflect.ValueOf(payload)
			found := false
			for i := range value.NumField() {
				if value.Type().Field(i).Tag.Get("yaml") != table {
					continue
				}
				found = true
				rows := value.Field(i)
				c.Assert(rows.Len(), tc.Equals, 1)
				c.Check(rows.Index(0).FieldByName("ID").Interface(), tc.Equals, id)
			}
			c.Check(found, tc.IsTrue, tc.Commentf("%s absent from payload", table))
		}
		for _, row := range []string{"{}", "{id: null}", "{id: ~}", "{id:}", "null"} {
			// Put the bad row after a valid row to check the reported index.
			data := []byte(fmt.Sprintf("%s:\n- {id: 0}\n- %s\n", table, row))
			payload, err := decode(version, data)
			c.Assert(err, tc.ErrorIs, coreerrors.NotValid, tc.Commentf("%s %s", table, row))
			c.Check(payload, tc.IsNil)
			c.Check(err, tc.ErrorMatches,
				fmt.Sprintf(`.*table %q row 1: id must be present and non-NULL.*`, table))
		}
	}
}

func (s *lookupPayloadSuite) TestLookupAliasesAndMerges(c *tc.C) {
	for _, decode := range []func(semversion.Number, []byte) (any, error){
		DecodePayload, DecodeControllerPayload,
	} {
		for _, row := range []string{
			"{id: &id 0}", "{id: *id}", "{<<: {id: 0}}",
		} {
			data := []byte("life:\n- {id: &id 42}\n- " + row + "\n")
			_, err := decode(semversion.MustParse("4.1.0"), data)
			c.Assert(err, tc.ErrorIsNil, tc.Commentf("%s", row))
		}
		for _, row := range []string{
			"{<<: {id: null}}", "{<<: {id: 42}, id: null}", "{id: *id}",
		} {
			data := []byte("ignored: &id null\nlife:\n- {id: 0}\n- " + row + "\n")
			_, err := decode(semversion.MustParse("4.1.0"), data)
			c.Assert(err, tc.ErrorIs, coreerrors.NotValid, tc.Commentf("%s", row))
			c.Check(err, tc.ErrorMatches, `.*table "life" row 1:.*`)
		}
		_, err := decode(semversion.MustParse("4.1.0"), []byte(
			"lookups: &rows [{id: 0}]\n<<: {life: *rows}\n"))
		c.Assert(err, tc.ErrorIsNil)
		_, err = decode(semversion.MustParse("4.1.0"), []byte(
			"lookups: &rows [{id: null}]\n<<: {life: *rows}\n"))
		c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
	}
}

func (s *lookupPayloadSuite) TestModelLookupNullableFields(c *tc.C) {
	for _, role := range []string{"null", "owner"} {
		data := []byte("secret_role:\n- {id: 0, role: " + role + "}\n" +
			"application_config:\n- {value: null}\n")
		payload, err := DecodePayload(semversion.MustParse("4.1.0"), data)
		c.Assert(err, tc.ErrorIsNil)
		out := payload.(v4_1_0.ModelExport)
		c.Assert(len(out.SecretRole), tc.Equals, 1)
		c.Check(out.SecretRole[0].ID, tc.Equals, int64(0))
		if role == "null" {
			c.Check(out.SecretRole[0].Role, tc.IsNil)
		} else {
			c.Assert(out.SecretRole[0].Role, tc.NotNil)
			c.Check(*out.SecretRole[0].Role, tc.Equals, role)
		}
		c.Assert(len(out.ApplicationConfig), tc.Equals, 1)
		c.Check(out.ApplicationConfig[0].Value, tc.IsNil)
		data, err = yaml.Marshal(out)
		c.Assert(err, tc.ErrorIsNil)
		decoded, err := DecodePayload(semversion.MustParse("4.1.0"), data)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(decoded, tc.DeepEquals, out)
	}
}

func (s *lookupPayloadSuite) TestControllerLookupNullableFields(c *tc.C) {
	for _, authType := range []string{"null", "oauth"} {
		data := []byte("auth_type:\n- {id: 0, type: " + authType + "}\n" +
			"cloud_credential_attribute:\n- {value: null}\n")
		payload, err := DecodeControllerPayload(semversion.MustParse("4.1.0"), data)
		c.Assert(err, tc.ErrorIsNil)
		out := payload.(ctrlv4_1_0.ControllerExport)
		c.Assert(len(out.AuthType), tc.Equals, 1)
		c.Check(out.AuthType[0].ID, tc.Equals, int64(0))
		if authType == "null" {
			c.Check(out.AuthType[0].Type, tc.IsNil)
		} else {
			c.Assert(out.AuthType[0].Type, tc.NotNil)
			c.Check(*out.AuthType[0].Type, tc.Equals, authType)
		}
		c.Assert(len(out.CloudCredentialAttribute), tc.Equals, 1)
		c.Check(out.CloudCredentialAttribute[0].Value, tc.IsNil)
		data, err = yaml.Marshal(out)
		c.Assert(err, tc.ErrorIsNil)
		decoded, err := DecodeControllerPayload(semversion.MustParse("4.1.0"), data)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(decoded, tc.DeepEquals, out)
	}
}

func (s *lookupPayloadSuite) TestModelTargetLocalLookupIDs(c *tc.C) {
	for _, row := range []string{"{}", "{id: null}", "{id: 0}"} {
		data := []byte("change_log_edit_type:\n- " + row +
			"\nchange_log_namespace:\n- " + row + "\n")
		_, err := DecodePayload(semversion.MustParse("4.1.0"), data)
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("%s", row))
	}
}

func (s *lookupPayloadSuite) TestLegacyLookupIDsUnchanged(c *tc.C) {
	for _, row := range []string{"{}", "{id: null}", "{id: 0}", "{id: 42}"} {
		data := []byte("life:\n- " + row + "\n")
		payload, err := DecodePayload(semversion.MustParse("4.0.12"), data)
		c.Assert(err, tc.ErrorIsNil, tc.Commentf("%s", row))
		out := payload.(v4_0_12.ModelExport)
		c.Assert(len(out.Life), tc.Equals, 1)
		switch row {
		case "{}", "{id: null}":
			c.Check(out.Life[0].ID, tc.IsNil)
		case "{id: 0}":
			c.Assert(out.Life[0].ID, tc.NotNil)
			c.Check(*out.Life[0].ID, tc.Equals, int64(0))
		case "{id: 42}":
			c.Assert(out.Life[0].ID, tc.NotNil)
			c.Check(*out.Life[0].ID, tc.Equals, int64(42))
		}
	}
}

func (s *lookupPayloadSuite) TestUnrelatedIDsAndScalarsUnchanged(c *tc.C) {
	for _, decode := range []func(semversion.Number, []byte) (any, error){
		DecodePayload, DecodeControllerPayload,
	} {
		for _, data := range []string{
			"change_log: [{id: null}]", "change_log: [{}]", "life: [{id: 0, value: null}]",
			"", "{}", "null", "life: null", "life: []",
		} {
			_, err := decode(semversion.MustParse("4.1.0"), []byte(data))
			c.Assert(err, tc.ErrorIsNil, tc.Commentf("%s", data))
		}
	}
}
