// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"testing"

	"github.com/juju/tc"
)

type removalSchemaSuite struct {
	schemaBaseSuite
}

func TestRemovalSchemaSuite(t *testing.T) {
	tc.Run(t, &removalSchemaSuite{})
}

func (s *removalSchemaSuite) SetUpTest(c *tc.C) {
	s.schemaBaseSuite.SetUpTest(c)
	s.applyDDL(c, ModelDDL())
}

// TestRemovalForceIsBoolean verifies that removal.force is declared as a
// non-null BOOLEAN defaulting to false, matching the Go bool that every writer
// binds and the reader scans.
func (s *removalSchemaSuite) TestRemovalForceIsBoolean(c *tc.C) {
	var typ, notNull, dflt string
	err := s.DB().QueryRowContext(c.Context(), `
SELECT type, CAST("notnull" AS TEXT), dflt_value
FROM   pragma_table_info('removal')
WHERE  name = 'force'`).Scan(&typ, &notNull, &dflt)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(typ, tc.Equals, "BOOLEAN")
	c.Check(notNull, tc.Equals, "1")
	c.Check(dflt, tc.Equals, "FALSE")
}

// TestRemovalForceStorage verifies that the default and explicit boolean
// values are persisted as the integers 0 and 1, which is the representation
// the legacy integer column used.
func (s *removalSchemaSuite) TestRemovalForceStorage(c *tc.C) {
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO removal (uuid, removal_type_id, entity_uuid, scheduled_for)
VALUES ('default', 0, 'e0', '2026-01-01T00:00:00Z')`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `
INSERT INTO removal (uuid, removal_type_id, entity_uuid, force, scheduled_for)
VALUES ('false', 0, 'e1', FALSE, '2026-01-01T00:00:00Z'),
       ('true', 0, 'e2', TRUE, '2026-01-01T00:00:00Z')`)
	c.Assert(err, tc.ErrorIsNil)

	rows, err := s.DB().QueryContext(c.Context(),
		`SELECT uuid, CAST(force AS TEXT), typeof(force) FROM removal`)
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	got := map[string][2]string{}
	for rows.Next() {
		var uuid, value, typ string
		c.Assert(rows.Scan(&uuid, &value, &typ), tc.ErrorIsNil)
		got[uuid] = [2]string{value, typ}
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
	c.Check(got, tc.DeepEquals, map[string][2]string{
		"default": {"0", "integer"},
		"false":   {"0", "integer"},
		"true":    {"1", "integer"},
	})
}
