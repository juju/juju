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
	startWithHeader := strings.HasPrefix(output,
		"child_table\t\tchild_column\tparent_column\tfk_id\tfk_seq\n",
	)
	c.Assert(startWithHeader, tc.Equals, true)
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
	startWithHeader := strings.HasPrefix(output,
		"child_table\t\tchild_column\tfk_id\tfk_seq\treference_count\n",
	)
	c.Assert(startWithHeader, tc.Equals, true)
}
