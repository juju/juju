// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database"
	databasetesting "github.com/juju/juju/internal/database/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type deleteDBSuite struct {
	databasetesting.DqliteSuite
}

func TestDeleteDBSuite(t *stdtesting.T) { tc.Run(t, &deleteDBSuite{}) }
func (s *deleteDBSuite) TestDeleteDBContentsOnEmptyDB(c *tc.C) {
	runner := s.TxnRunner()

	err := runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return deleteDBContents(ctx, tx, loggertesting.WrapCheckLog(c))
	})
	c.Assert(err, tc.IsNil)
}

func (s *deleteDBSuite) TestDeleteDBContentsOnControllerDB(c *tc.C) {
	runner, db := s.OpenDBForNamespace(c, "controller-foo", false)
	logger := loggertesting.WrapCheckLog(c)

	// This test isn't necessarily, as you can't delete the controller database
	// contents, but adds more validation to the function.

	err := database.NewDBMigration(
		runner, logger, schema.ControllerDDL()).Apply(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return deleteDBContents(ctx, tx, logger)
	})
	c.Assert(err, tc.IsNil)

	s.ensureEmpty(c, db)
}

func (s *deleteDBSuite) TestDeleteDBContentsOnModelDB(c *tc.C) {
	runner, db := s.OpenDBForNamespace(c, "model-foo", false)

	logger := loggertesting.WrapCheckLog(c)

	err := database.NewDBMigration(
		runner, logger, schema.ModelDDL()).Apply(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = runner.StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return deleteDBContents(ctx, tx, logger)
	})
	c.Assert(err, tc.IsNil)

	s.ensureEmpty(c, db)
}

func (s *deleteDBSuite) ensureEmpty(c *tc.C, db *sql.DB) {
	schemaStmt := `SELECT COUNT(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%';`
	var count int
	err := db.QueryRow(schemaStmt).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}
