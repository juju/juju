// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build dqlite && linux

package schema

import (
	"testing"

	"github.com/juju/tc"

	databasetesting "github.com/juju/juju/internal/database/testing"
)

type nullableBooleanDqliteSuite struct {
	databasetesting.DqliteSuite
}

func TestNullableBooleanDqliteSuite(t *testing.T) {
	tc.Run(t, &nullableBooleanDqliteSuite{})
}

func (s *nullableBooleanDqliteSuite) TestWorkaroundStillRequired(c *tc.C) {
	_, err := s.DB().ExecContext(c.Context(), `CREATE TABLE nullable_boolean (value BOOLEAN)`)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(), `INSERT INTO nullable_boolean (value) VALUES (NULL)`)
	c.Assert(err, tc.ErrorIsNil)

	var storedNull bool
	var result any
	err = s.DB().QueryRowContext(c.Context(),
		`SELECT value IS NULL, value FROM nullable_boolean`,
	).Scan(&storedNull, &result)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storedNull, tc.IsTrue)
	c.Check(result, tc.Equals, false, tc.Commentf(
		"dqlite NULL BOOLEAN behaviour changed; remove the workarounds for canonical/dqlite#883",
	))
}
