// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pragma_test

import (
	"database/sql"
	"fmt"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/database/pragma"
	databasetesting "github.com/juju/juju/internal/database/testing"
)

type optimizeSuite struct {
	databasetesting.DqliteSuite
}

func TestOptimizeSuite(t *stdtesting.T) {
	tc.Run(t, &optimizeSuite{})
}

func (s *optimizeSuite) TestOptimizeEmptyDatabase(c *tc.C) {
	db := s.DB()

	statements, err := pragma.OptimizeDryRun(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statements, tc.HasLen, 0)

	err = pragma.Optimize(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(analysedIndexes(c, db), tc.HasLen, 0)
}

func (s *optimizeSuite) TestOptimizeAnalysesTables(c *tc.C) {
	db := s.DB()
	createTable(c, db, "foo")
	insertRows(c, db, "foo", 0, 100)

	statements, err := pragma.OptimizeDryRun(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statements, tc.DeepEquals, []string{`ANALYZE "main"."foo"`})

	err = pragma.Optimize(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(analysedIndexes(c, db), tc.DeepEquals, []string{"foo_value_idx"})

	// With the statistics gathered, there is nothing left to do.
	statements, err = pragma.OptimizeDryRun(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statements, tc.HasLen, 0)
}

func (s *optimizeSuite) TestOptimizeDryRunDoesNotWrite(c *tc.C) {
	db := s.DB()
	createTable(c, db, "foo")
	insertRows(c, db, "foo", 0, 100)

	statements, err := pragma.OptimizeDryRun(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statements, tc.HasLen, 1)

	// Reporting the work must not do any of it.
	c.Check(analysedIndexes(c, db), tc.HasLen, 0)
}

func (s *optimizeSuite) TestOptimizeSkipsEmptyTables(c *tc.C) {
	db := s.DB()
	createTable(c, db, "foo")

	err := pragma.Optimize(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(analysedIndexes(c, db), tc.HasLen, 0)
}

// TestOptimizeConsidersTablesTheConnectionHasNotUsed covers the reason that we
// pass an explicit mask rather than using the SQLite default. By default only
// the tables whose statistics the connection has already used are considered,
// which would mean each pooled connection only ever seeing a fraction of the
// workload.
func (s *optimizeSuite) TestOptimizeConsidersTablesTheConnectionHasNotUsed(c *tc.C) {
	_, db := s.OpenDBForNamespace(c, "optimize", true)
	createTable(c, db, "foo")
	insertRows(c, db, "foo", 0, 100)

	err := pragma.Optimize(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)

	// Grow the table by more than 10-fold, which is what makes the gathered
	// statistics stale.
	insertRows(c, db, "foo", 100, 5000)

	// This connection has never touched the table, but must still see that it
	// needs analysing.
	_, other := s.OpenDBForNamespace(c, "optimize", true)

	statements, err := pragma.OptimizeDryRun(c.Context(), other)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statements, tc.DeepEquals, []string{`ANALYZE "main"."foo"`})

	err = pragma.Optimize(c.Context(), other)
	c.Assert(err, tc.ErrorIsNil)

	statements, err = pragma.OptimizeDryRun(c.Context(), other)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(statements, tc.HasLen, 0)
}

func createTable(c *tc.C, db *sql.DB, name string) {
	_, err := db.ExecContext(c.Context(), fmt.Sprintf(`
CREATE TABLE %s (
	id    INTEGER PRIMARY KEY,
	value TEXT NOT NULL
)`, name))
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.ExecContext(c.Context(),
		fmt.Sprintf("CREATE INDEX %s_value_idx ON %s (value)", name, name))
	c.Assert(err, tc.ErrorIsNil)
}

func insertRows(c *tc.C, db *sql.DB, name string, from, count int) {
	for i := from; i < from+count; i++ {
		_, err := db.ExecContext(c.Context(),
			fmt.Sprintf("INSERT INTO %s (id, value) VALUES (?, ?)", name), i, fmt.Sprintf("value-%d", i%10))
		c.Assert(err, tc.ErrorIsNil)
	}
}

// analysedIndexes returns the names of the indexes that the query planner has
// gathered statistics for.
func analysedIndexes(c *tc.C, db *sql.DB) []string {
	// The statistics table only comes into existence the first time that
	// something is actually analysed.
	var analysed int
	err := db.QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM sqlite_master WHERE name = 'sqlite_stat1'").Scan(&analysed)
	c.Assert(err, tc.ErrorIsNil)

	if analysed == 0 {
		return nil
	}

	rows, err := db.QueryContext(c.Context(),
		"SELECT idx FROM sqlite_stat1 WHERE idx IS NOT NULL ORDER BY idx")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var indexes []string
	for rows.Next() {
		var index string
		c.Assert(rows.Scan(&index), tc.ErrorIsNil)
		indexes = append(indexes, index)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)

	return indexes
}
