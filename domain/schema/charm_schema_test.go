// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"testing"

	"github.com/juju/tc"
)

type charmSchemaSuite struct {
	schemaBaseSuite
}

func TestCharmSchemaSuite(t *testing.T) {
	tc.Run(t, &charmSchemaSuite{})
}

func (s *charmSchemaSuite) SetUpTest(c *tc.C) {
	s.schemaBaseSuite.SetUpTest(c)
	s.applyDDL(c, ModelDDL())
	s.assertExecSQL(c, `
INSERT INTO charm (uuid, reference_name) VALUES ('charm-uuid', 'foo')`)
}

// TestCharmContainerMountRequiresContainerKey verifies that a mount must name
// the container it belongs to. The key is part of the primary key and of the
// foreign key to charm_container, which a NULL would silently bypass.
func (s *charmSchemaSuite) TestCharmContainerMountRequiresContainerKey(c *tc.C) {
	s.assertExecSQL(c, `
INSERT INTO charm_container (charm_uuid, "key") VALUES ('charm-uuid', 'c1')`)
	s.assertExecSQL(c, `
INSERT INTO charm_container_mount (array_index, charm_uuid, charm_container_key)
VALUES (0, 'charm-uuid', 'c1')`)

	s.assertExecSQLError(c, `
INSERT INTO charm_container_mount (array_index, charm_uuid, charm_container_key)
VALUES (1, 'charm-uuid', NULL)`,
		`NOT NULL constraint failed: charm_container_mount.charm_container_key`)
}

// TestCharmManifestBaseIdentity verifies that a manifest base is identified by
// its position in the manifest alone. The descriptive columns may be NULL, so
// they are not part of the key, and a second base at the same position is
// rejected even when its descriptive columns differ.
func (s *charmSchemaSuite) TestCharmManifestBaseIdentity(c *tc.C) {
	rows, err := s.DB().QueryContext(c.Context(), `
SELECT name FROM pragma_table_info('charm_manifest_base') WHERE pk > 0 ORDER BY pk`)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()
	var pk []string
	for rows.Next() {
		var name string
		c.Assert(rows.Scan(&name), tc.ErrorIsNil)
		pk = append(pk, name)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Check(pk, tc.DeepEquals, []string{"charm_uuid", "array_index", "nested_array_index"})

	s.assertExecSQL(c, `
INSERT INTO charm_manifest_base (charm_uuid, array_index, nested_array_index, os_id, track, risk, branch, architecture_id)
VALUES ('charm-uuid', 0, 0, 0, NULL, 'stable', NULL, 0),
       ('charm-uuid', 0, 1, 0, NULL, 'stable', NULL, 1)`)
	s.assertExecSQLError(c, `
INSERT INTO charm_manifest_base (charm_uuid, array_index, nested_array_index, os_id, track, risk, branch, architecture_id)
VALUES ('charm-uuid', 0, 0, 0, '24.04', 'edge', 'b', 1)`,
		`UNIQUE constraint failed: charm_manifest_base.charm_uuid, charm_manifest_base.array_index, charm_manifest_base.nested_array_index`)
}
