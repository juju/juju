//go:build !dqlite && cgo && (sqlite_trace || trace)

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package app

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/database/client"
)

type sqliteTraceSuite struct{}

func TestSqliteTraceSuite(t *testing.T) {
	tc.Run(t, &sqliteTraceSuite{})
}

func (s *sqliteTraceSuite) TestFullTableScanIsLogged(c *tc.C) {
	var logs []string
	db := s.openDB(c, &logs)

	_, err := db.ExecContext(c.Context(), `
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    name TEXT,
    age INTEGER
)`)
	c.Assert(err, tc.ErrorIsNil)

	for i := range 10 {
		_, err = db.ExecContext(
			c.Context(),
			"INSERT INTO users (name, age) VALUES (?, ?)",
			fmt.Sprintf("user%d", i),
			i%3,
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	rows, err := db.QueryContext(
		c.Context(),
		"SELECT * FROM users WHERE age = ?",
		2,
	)
	c.Assert(err, tc.ErrorIsNil)
	consumeRows(c, rows)

	found := false
	for _, log := range logs {
		if strings.Contains(log, "full table scan") &&
			strings.Contains(log, "SELECT * FROM users WHERE age = 2") {
			found = true
		}
	}
	c.Check(found, tc.IsTrue, tc.Commentf("logs: %#v", logs))
}

func (s *sqliteTraceSuite) TestCoveringIndexScanIsNotLogged(c *tc.C) {
	var logs []string
	db := s.openDB(c, &logs)

	_, err := db.ExecContext(c.Context(), `
CREATE TABLE schema (
    id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    version INTEGER NOT NULL,
    hash TEXT NOT NULL,
    updated_at DATETIME NOT NULL
)`)
	c.Assert(err, tc.ErrorIsNil)

	_, err = db.ExecContext(
		c.Context(),
		"CREATE INDEX idx_schema_version_hash ON schema (version, hash)",
	)
	c.Assert(err, tc.ErrorIsNil)

	for i := range 10 {
		_, err = db.ExecContext(
			c.Context(),
			"INSERT INTO schema (version, hash, updated_at) VALUES (?, ?, strftime('%s'))",
			i,
			fmt.Sprintf("hash%d", i),
		)
		c.Assert(err, tc.ErrorIsNil)
	}

	rows, err := db.QueryContext(
		c.Context(),
		"SELECT version, hash FROM schema ORDER BY version",
	)
	c.Assert(err, tc.ErrorIsNil)
	consumeVersionHashRows(c, rows)

	for _, log := range logs {
		c.Check(log, tc.Not(tc.Contains), "SELECT version, hash FROM schema")
	}
}

func (s *sqliteTraceSuite) openDB(c *tc.C, logs *[]string) *sql.DB {
	app, err := New(c.MkDir(),
		WithTracing(client.LogWarn),
		WithLogFunc(func(_ client.LogLevel, msg string, args ...any) {
			*logs = append(*logs, fmt.Sprintf(msg, args...))
		}),
	)
	c.Assert(err, tc.ErrorIsNil)

	db, err := app.Open(c.Context(), ":memory:")
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() {
		c.Check(db.Close(), tc.ErrorIsNil)
	})
	return db
}

func consumeRows(c *tc.C, rows *sql.Rows) {
	defer func() {
		c.Check(rows.Close(), tc.ErrorIsNil)
	}()

	for rows.Next() {
		var id int
		var name string
		var age int
		c.Assert(rows.Scan(&id, &name, &age), tc.ErrorIsNil)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
}

func consumeVersionHashRows(c *tc.C, rows *sql.Rows) {
	defer func() {
		c.Check(rows.Close(), tc.ErrorIsNil)
	}()

	for rows.Next() {
		var version int
		var hash string
		c.Assert(rows.Scan(&version, &hash), tc.ErrorIsNil)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)
}
