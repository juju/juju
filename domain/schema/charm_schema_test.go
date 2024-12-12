// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	gc "gopkg.in/check.v1"
)

type charmSchemaSuite struct {
	schemaBaseSuite
}

var _ = gc.Suite(&charmSchemaSuite{})

func (s *charmSchemaSuite) TestCharmSequence(c *gc.C) {
	s.applyDDL(c, ModelDDL())

	// Insert the first sequence, then every other one is monotonically
	// increasing.

	s.assertExecSQL(c, "INSERT INTO sequence_charm_local (source_id, reference_name, sequence) VALUES ('1', 'foo', 1)")
	s.assertExecSQL(c, "INSERT INTO charm (uuid, source_id, reference_name, revision) VALUES ('abc', '1', 'foo', 1)")

	s.assertExecSQL(c, "UPDATE sequence_charm_local SET sequence=2 WHERE source_id='1' AND reference_name='foo'")
	s.assertExecSQL(c, "INSERT INTO charm (uuid, source_id, reference_name, revision) VALUES ('def', '1', 'foo', 2)")

	s.assertExecSQL(c, "UPDATE sequence_charm_local SET sequence=3 WHERE source_id='1' AND reference_name='foo'")
	s.assertExecSQL(c, "INSERT INTO charm (uuid, source_id, reference_name, revision) VALUES ('ghi', '1', 'foo', 3)")

	// Ensure we can't go backwards.

	s.assertExecSQLError(c, "UPDATE sequence_charm_local SET sequence=2 WHERE source_id='1' AND reference_name='foo'", "sequence number must monotonically increase")
}
