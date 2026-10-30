// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/juju/tc"

	controllerschema "github.com/juju/juju/core/database/schema"
	controllerexport "github.com/juju/juju/domain/export/types/controller/v4_1_0"
	modelexport "github.com/juju/juju/domain/export/types/v4_1_0"
)

type lookupPrimaryKeySchemaSuite struct {
	schemaBaseSuite
}

func TestLookupPrimaryKeySchemaSuite(t *testing.T) {
	tc.Run(t, &lookupPrimaryKeySchemaSuite{})
}

func (s *lookupPrimaryKeySchemaSuite) TestModelLookupPrimaryKeys(c *tc.C) {
	s.checkLookupPrimaryKeys(c, ModelDDL(), modelLookupPrimaryKeyTables, []string{
		"relation_status_type", "removal_type", "storage_pool_origin",
	}, reflect.TypeFor[modelexport.ModelExport]())
}

func (s *lookupPrimaryKeySchemaSuite) TestControllerLookupPrimaryKeys(c *tc.C) {
	s.checkLookupPrimaryKeys(c, ControllerDDL(), controllerLookupPrimaryKeyTables, []string{
		"model_migration_import_phase_type", "model_migration_phase",
		"object_store_backend_type", "secret_backend_origin",
	}, reflect.TypeFor[controllerexport.ControllerExport]())
}

func (s *lookupPrimaryKeySchemaSuite) checkLookupPrimaryKeys(
	c *tc.C, ddl *controllerschema.Schema, changed, alreadyNotNull []string, payload reflect.Type,
) {
	s.applyDDL(c, ddl)
	rows, err := s.DB().QueryContext(c.Context(), `
SELECT m.name, p.name, p."notnull"
FROM sqlite_master AS m
JOIN pragma_table_info(m.name) AS p
WHERE m.type = 'table' AND p.type = 'INT' AND p.pk = 1
AND (SELECT COUNT(*) FROM pragma_table_info(m.name) AS k WHERE k.pk > 0) = 1
ORDER BY m.name`)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { c.Check(rows.Close(), tc.ErrorIsNil) }()
	var tables []string
	for rows.Next() {
		var table, column string
		var notNull int
		c.Assert(rows.Scan(&table, &column, &notNull), tc.ErrorIsNil)
		c.Check(column, tc.Equals, "id", tc.Commentf("table %s", table))
		c.Check(notNull, tc.Equals, 1, tc.Commentf("table %s", table))
		tables = append(tables, table)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
	expected := append(slices.Clone(changed), alreadyNotNull...)
	slices.Sort(expected)
	c.Check(tables, tc.DeepEquals, expected)

	for _, table := range changed {
		var field reflect.StructField
		var found bool
		for candidate := range payload.Fields() {
			if candidate.Tag.Get("yaml") == table {
				field, found = candidate, true
				break
			}
		}
		c.Assert(found, tc.IsTrue, tc.Commentf("export table %s", table))
		id, found := field.Type.Elem().FieldByName("ID")
		c.Assert(found, tc.IsTrue, tc.Commentf("export table %s", table))
		c.Check(id.Type, tc.Equals, reflect.TypeFor[int64](), tc.Commentf("export table %s", table))

		var count, invalid int
		query := fmt.Sprintf(`SELECT COUNT(*), COUNT(*) FILTER (WHERE typeof(id) != 'integer') FROM %q`, table)
		c.Assert(s.DB().QueryRowContext(c.Context(), query).Scan(&count, &invalid), tc.ErrorIsNil)
		c.Check(invalid, tc.Equals, 0, tc.Commentf("seed table %s", table))
		c.Assert(count, tc.GreaterThan, 0, tc.Commentf("seed table %s", table))
		if table != "change_log_namespace" && table != "change_log_edit_type" {
			var first, last int64
			query = fmt.Sprintf(`SELECT MIN(id), MAX(id) FROM %q`, table)
			c.Assert(s.DB().QueryRowContext(c.Context(), query).Scan(&first, &last), tc.ErrorIsNil)
			c.Check(first, tc.Equals, int64(0), tc.Commentf("seed table %s", table))
			c.Check(last, tc.Equals, int64(count-1), tc.Commentf("seed table %s", table))
		}
		// Updating an existing seed supplies valid values for every other column.
		query = fmt.Sprintf(`UPDATE %q SET id = NULL WHERE id = (SELECT MIN(id) FROM %q)`, table, table)
		s.assertExecSQLError(c, query, fmt.Sprintf(`NOT NULL constraint failed: %s.id`, table))
		var after int
		c.Assert(s.DB().QueryRowContext(c.Context(), fmt.Sprintf(`SELECT COUNT(*) FROM %q`, table)).Scan(&after),
			tc.ErrorIsNil)
		c.Check(after, tc.Equals, count)
	}

	var editTypeIDs []int64
	seedRows, err := s.DB().QueryContext(c.Context(), `SELECT id FROM change_log_edit_type ORDER BY id`)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { c.Check(seedRows.Close(), tc.ErrorIsNil) }()
	for seedRows.Next() {
		var id int64
		c.Assert(seedRows.Scan(&id), tc.ErrorIsNil)
		editTypeIDs = append(editTypeIDs, id)
	}
	c.Assert(seedRows.Err(), tc.ErrorIsNil)
	c.Check(editTypeIDs, tc.DeepEquals, []int64{1, 2, 4})

	// INTEGER PRIMARY KEY retains its rowid allocation semantics.
	s.assertExecSQL(c, `INSERT INTO change_log_namespace (id, namespace) VALUES (999, 'lookup-test')`)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var id int64
		err := tx.QueryRowContext(ctx, `
INSERT INTO change_log (id, edit_type_id, namespace_id, changed)
VALUES (NULL, 1, 999, 'lookup-test') RETURNING id`).Scan(&id)
		if err != nil {
			return err
		}
		c.Check(id, tc.GreaterThan, int64(0))
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

var modelLookupPrimaryKeyTables = []string{
	"agent_stream", "architecture", "block_command_type", "block_device_provenance",
	"change_log_edit_type", "change_log_namespace", "charm_config_type", "charm_provenance",
	"charm_relation_role", "charm_relation_scope", "charm_resource_kind", "charm_run_as_kind",
	"charm_source", "charm_storage_kind", "container_type", "hash_kind",
	"ip_address_config_type", "ip_address_origin", "ip_address_scope", "ip_address_type",
	"k8s_pod_status_value", "life", "link_layer_device_type", "machine_cloud_instance_status_value",
	"machine_placement_scope", "machine_status_value", "network_address_scope", "operation_task_status_value",
	"os", "password_hash_algorithm", "protocol", "resolve_mode", "resource_origin_type",
	"resource_retrieved_by_type", "resource_state", "secret_grant_scope_type", "secret_grant_subject_type",
	"secret_role", "secret_rotate_policy", "ssh_key_algorithm_type", "storage_filesystem_status_value",
	"storage_kind", "storage_provision_scope", "storage_volume_device_type", "storage_volume_status_value",
	"unit_agent_status_value", "virtual_port_type", "workload_status_value",
}

var controllerLookupPrimaryKeyTables = []string{
	"architecture", "auth_type", "change_log_edit_type", "change_log_namespace", "cloud_type",
	"lease_type", "life", "model_type", "object_store_drain_phase_type", "password_hash_algorithm",
	"permission_access_type", "permission_object_access", "permission_object_type", "secret_backend_type",
	"ssh_fingerprint_hash_algorithm", "ssh_key_algorithm_type", "upgrade_state_type",
}
