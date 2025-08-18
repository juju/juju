// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	databasetesting "github.com/juju/juju/internal/database/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type patchSuite struct {
	testhelpers.IsolationSuite

	tx *MockTx
}

func TestPatchSuite(t *testing.T) {
	tc.Run(t, &patchSuite{})
}

func (s *patchSuite) TestPatchHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	patch := MakePatch("SELECT 1")
	c.Assert(patch, tc.NotNil)
	c.Assert(patch.hash, tc.Equals, "4ATr1bVTKkuFmEpi+K1IqBqjRgwcoHcB84YTXXLN7PU=")
}

func (s *patchSuite) TestPatchHashWithSpaces(c *tc.C) {
	defer s.setupMocks(c).Finish()

	patch := MakePatch(`
                SELECT 1
`)
	c.Assert(patch, tc.NotNil)
	c.Assert(patch.hash, tc.Equals, "4ATr1bVTKkuFmEpi+K1IqBqjRgwcoHcB84YTXXLN7PU=")
}

func (s *patchSuite) TestPatchRun(c *tc.C) {
	defer s.setupMocks(c).Finish()

	patch := MakePatch("SELECT * FROM schema_master", 1, 2, "a")

	s.tx.EXPECT().ExecContext(gomock.Any(), "SELECT * FROM schema_master", 1, 2, "a").Return(nil, nil)

	err := patch.run(c.Context(), s.tx)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *patchSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.tx = NewMockTx(ctrl)

	return ctrl
}

type schemaSuite struct {
	databasetesting.DqliteSuite
}

func TestSchemaSuite(t *testing.T) {
	tc.Run(t, &schemaSuite{})
}

func (s *schemaSuite) TestSchemaAdd(c *tc.C) {
	schema := New(
		MakePatch("SELECT 1"),
		MakePatch("SELECT 2"),
	)
	c.Check(schema.Len(), tc.Equals, 2)

	schema.Add(MakePatch("SELECT 3"))
	c.Check(schema.Len(), tc.Equals, 3)
	schema.Add(MakePatch("SELECT 4"))
	c.Check(schema.Len(), tc.Equals, 4)
}

func (s *schemaSuite) TestEnsureWithNoPatches(c *tc.C) {
	schema := New()
	current, err := schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 0, Post: 0})
}

func (s *schemaSuite) TestSchemaRunMultipleTimes(c *tc.C) {
	schema := New(
		MakePatch("CREATE TEMP TABLE foo (id INTEGER PRIMARY KEY);"),
		MakePatch("CREATE TEMP TABLE bar (id INTEGER PRIMARY KEY);"),
	)
	current, err := schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 0, Post: 2})

	schema = New(
		MakePatch("CREATE TEMP TABLE foo (id INTEGER PRIMARY KEY);"),
		MakePatch("CREATE TEMP TABLE bar (id INTEGER PRIMARY KEY);"),
	)
	current, err = schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 2, Post: 2})
}

func (s *schemaSuite) TestSchemaRunMultipleTimesWithAdditions(c *tc.C) {
	schema := New(
		MakePatch("CREATE TEMP TABLE foo (id INTEGER PRIMARY KEY);"),
		MakePatch("CREATE TEMP TABLE bar (id INTEGER PRIMARY KEY);"),
	)
	current, err := schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 0, Post: 2})

	schema = New(
		MakePatch("CREATE TEMP TABLE foo (id INTEGER PRIMARY KEY);"),
		MakePatch("CREATE TEMP TABLE bar (id INTEGER PRIMARY KEY);"),
		MakePatch("CREATE TEMP TABLE baz (id INTEGER PRIMARY KEY);"),
	)
	current, err = schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 2, Post: 3})
}

func (s *schemaSuite) TestEnsure(c *tc.C) {
	schema := New(
		MakePatch("CREATE TEMP TABLE foo (id INTEGER PRIMARY KEY);"),
		MakePatch("CREATE TEMP TABLE bar (id INTEGER PRIMARY KEY);"),
	)
	current, err := schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 0, Post: 2})
}

func (s *schemaSuite) TestEnsureIdempotent(c *tc.C) {
	schema := New(
		MakePatch("CREATE TEMP TABLE foo (id INTEGER PRIMARY KEY);"),
		MakePatch("CREATE TEMP TABLE bar (id INTEGER PRIMARY KEY);"),
	)
	current, err := schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 0, Post: 2})

	current, err = schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 2, Post: 2})
}

func (s *schemaSuite) TestEnsureTwiceWithAdditionalChanges(c *tc.C) {
	schema := New(
		MakePatch("CREATE TEMP TABLE foo (id INTEGER PRIMARY KEY);"),
		MakePatch("CREATE TEMP TABLE bar (id INTEGER PRIMARY KEY);"),
	)
	current, err := schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 0, Post: 2})

	schema.Add(MakePatch("CREATE TEMP TABLE baz (id INTEGER PRIMARY KEY);"))

	current, err = schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 2, Post: 3})

	schema.Add(MakePatch("CREATE TEMP TABLE alice (id INTEGER PRIMARY KEY);"))

	current, err = schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 3, Post: 4})
}

func (s *schemaSuite) TestEnsureHashBreaks(c *tc.C) {
	schema := New(
		MakePatch("CREATE TEMP TABLE foo (id INTEGER PRIMARY KEY);"),
		MakePatch("CREATE TEMP TABLE bar (id INTEGER PRIMARY KEY);"),
	)
	current, err := schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 0, Post: 2})

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE schema SET hash = 'blah' WHERE version=2;")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	schema.Add(MakePatch("CREATE TEMP TABLE baz (id INTEGER PRIMARY KEY);"))

	_, err = schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.ErrorMatches, `querying current schema version: hash mismatch for version 2`)
}

func (s *schemaSuite) TestHookIsApplied(c *tc.C) {
	schema := New(
		MakePatch("this would fail if the hook is not applied"),
	)
	schema.Hook(func(int, string) (string, error) {
		return "CREATE TABLE bar (id INTEGER PRIMARY KEY);", nil
	})
	c.Log(&schema.hook)

	current, err := schema.Ensure(c.Context(), s.TxnRunner())
	c.Assert(err, tc.IsNil)
	c.Assert(current, tc.DeepEquals, ChangeSet{Current: 0, Post: 1})

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, "SELECT * FROM bar")
		return row.Err()
	})
	c.Assert(err, tc.ErrorIsNil)
}
