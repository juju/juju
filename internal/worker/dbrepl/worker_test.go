// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbrepl

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/schema"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/database/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type dbReplSuite struct {
	testing.DqliteSuite
}

func headerFields(output string) []string {
	firstLine, _, _ := strings.Cut(output, "\n")
	return strings.Fields(firstLine)
}

func TestDbReplSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &dbReplSuite{})
	})
}

func (s *dbReplSuite) SetUpTest(c *tc.C) {
	s.DqliteSuite.SetUpTest(c)

	s.ApplyDDL(c, &schematesting.SchemaApplier{
		Schema: schema.ModelDDL(),
	})
}

func (s *dbReplSuite) TestChangeStreamLatestRows(c *tc.C) {
	// Interleave keys and namespaces, and make timestamps disagree with ID
	// order so every displayed field must come from the maximum-ID row.
	s.seedChangeStream(c, []changeStreamRow{
		{1, 1, 1000001, "shared", "2025-01-01T00:00:09Z"},
		{2, 1, 1000001, "other", "2025-01-01T00:00:08Z"},
		{3, 1, 1000002, "shared", "2025-01-01T00:00:07Z"},
		{4, 2, 1000001, "shared", "2025-01-01T00:00:06Z"},
		{5, 4, 1000002, "shared", "2025-01-01T00:00:05Z"},
		{6, 2, 1000001, "other", "2025-01-01T00:00:04Z"},
		{7, 4, 1000001, "shared", "2025-01-01T00:00:03Z"},
	})

	c.Check(s.changeStreamOutput(c), tc.DeepEquals, [][]string{
		{"id", "edit_type", "namespace", "changed", "created_at"},
		{"5", "delete", "repl-test-b", "shared", "2025-01-01T00:00:05Z"},
		{"6", "update", "repl-test-a", "other", "2025-01-01T00:00:04Z"},
		{"7", "delete", "repl-test-a", "shared", "2025-01-01T00:00:03Z"},
	})
}

func (s *dbReplSuite) TestChangeStreamEmpty(c *tc.C) {
	s.seedChangeStream(c, nil)

	c.Check(s.changeStreamOutput(c), tc.DeepEquals, [][]string{
		{"id", "edit_type", "namespace", "changed", "created_at"},
	})
}

func (s *dbReplSuite) TestChangeStreamLowerBound(c *tc.C) {
	// The command's existing bound excludes zero and negative IDs, including
	// keys with no positive changes, but retains the first positive ID.
	s.seedChangeStream(c, []changeStreamRow{
		{-2, 1, 1000001, "negative", "2025-01-01T00:00:05Z"},
		{-1, 1, 1000001, "shared", "2025-01-01T00:00:04Z"},
		{0, 2, 1000001, "shared", "2025-01-01T00:00:03Z"},
		{1, 4, 1000001, "shared", "2025-01-01T00:00:02Z"},
		{2, 1, 1000002, "positive", "2025-01-01T00:00:01Z"},
	})

	c.Check(s.changeStreamOutput(c), tc.DeepEquals, [][]string{
		{"id", "edit_type", "namespace", "changed", "created_at"},
		{"1", "delete", "repl-test-a", "shared", "2025-01-01T00:00:02Z"},
		{"2", "create", "repl-test-b", "positive", "2025-01-01T00:00:01Z"},
	})
}

func (s *dbReplSuite) TestChangeStreamOnlyExcludedRows(c *tc.C) {
	s.seedChangeStream(c, []changeStreamRow{
		{-1, 1, 1000001, "negative", "2025-01-01T00:00:02Z"},
		{0, 2, 1000001, "zero", "2025-01-01T00:00:01Z"},
	})

	c.Check(s.changeStreamOutput(c), tc.DeepEquals, [][]string{
		{"id", "edit_type", "namespace", "changed", "created_at"},
	})
}

func (s *dbReplSuite) TestForeignKeysList(c *tc.C) {
	// Use a bytes.Buffer to capture stdout output
	var stdout bytes.Buffer

	worker := dbReplWorker{
		cfg: WorkerConfig{
			Stdout: &stdout,
		},
		currentDB: s.DqliteSuite.TxnRunner(),
	}
	args := []string{"unit", "uuid"}
	worker.execForeignKeysList(c.Context(), args)

	// Check that the output contains expected content
	output := stdout.String()
	c.Check(output, tc.Not(tc.Equals), "No foreign key references found for table \"unit\" column \"uuid\"\n")
	c.Assert(headerFields(output), tc.DeepEquals, []string{
		"child_table", "child_column", "parent_column", "fk_id", "fk_seq",
	})
}

func (s *dbReplSuite) TestForeignKeysListIdentifier(c *tc.C) {
	// Use a bytes.Buffer to capture stdout output
	var stdout bytes.Buffer

	worker := dbReplWorker{
		cfg: WorkerConfig{
			Stdout: &stdout,
		},
		currentDB: s.DqliteSuite.TxnRunner(),
	}
	args := []string{"unit", "uuid", "identifier"}
	worker.execForeignKeysList(c.Context(), args)

	// Check that the output contains expected content
	output := stdout.String()
	c.Check(output, tc.Not(tc.Equals), "No foreign key references found for table \"unit\" column \"uuid\"\n")
	c.Assert(headerFields(output), tc.DeepEquals, []string{
		"child_table", "child_column", "fk_id", "fk_seq", "reference_count",
	})
}

type changeStreamRow struct {
	id          int
	editTypeID  int
	namespaceID int
	changed     string
	createdAt   string
}

func (s *dbReplSuite) seedChangeStream(c *tc.C, changes []changeStreamRow) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM change_log"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO change_log_namespace (id, namespace) VALUES
    (1000001, 'repl-test-a'), (1000002, 'repl-test-b')
`); err != nil {
			return err
		}
		for _, change := range changes {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO change_log (id, edit_type_id, namespace_id, changed, created_at)
VALUES (?, ?, ?, ?, ?)
`, change.id, change.editTypeID, change.namespaceID, change.changed, change.createdAt); err != nil {
				return err
			}
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *dbReplSuite) changeStreamOutput(c *tc.C) [][]string {
	var stdout bytes.Buffer
	worker := dbReplWorker{
		cfg: WorkerConfig{
			Stdout: &stdout,
			Logger: loggertesting.WrapCheckLog(c),
		},
		currentDB: s.TxnRunner(),
	}
	worker.changeStream(c.Context())

	var lines [][]string
	for line := range strings.SplitSeq(strings.TrimSpace(stdout.String()), "\n") {
		lines = append(lines, strings.Fields(line))
	}
	return lines
}
