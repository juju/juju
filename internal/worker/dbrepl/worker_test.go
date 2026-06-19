// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbrepl

import (
	"bytes"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/goleak"

	"github.com/juju/juju/domain/schema"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/database/testing"
)

type dbReplSuite struct {
	testing.DqliteSuite
}

func headerFields(output string) []string {
	firstLine, _, _ := strings.Cut(output, "\n")
	return strings.Fields(firstLine)
}

func TestDbReplSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &dbReplSuite{})
}

func (s *dbReplSuite) SetUpTest(c *tc.C) {
	s.DqliteSuite.SetUpTest(c)

	s.ApplyDDL(c, &schematesting.SchemaApplier{
		Schema: schema.ModelDDL(),
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
