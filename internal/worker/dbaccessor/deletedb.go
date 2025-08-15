// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
)

type sqliteSchema struct {
	Type string
	Name string
}

// deleteDBContents deletes all the contents of the database, this is very
// dynamic. It will drop all tables, views, triggers, and indexes. This can
// be replaced with DROP DATABASE once it's supported by dqlite.
func deleteDBContents(ctx context.Context, tx *sql.Tx, logger logger.Logger) error {
	// We should ignore any name that starts with sqlite_ as they are internal
	// tables, indexes and triggers.
	schemaStmt := `SELECT type, name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%';`

	var (
		indexes  = make(map[string]struct{})
		triggers = make(map[string]struct{})
		views    = make(map[string]struct{})
		tables   = make(map[string]struct{})
	)

	rows, err := tx.QueryContext(ctx, schemaStmt)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var schema sqliteSchema
		err := rows.Scan(&schema.Type, &schema.Name)
		if err != nil {
			return errors.Trace(err)
		}

		name := schema.Name
		switch schema.Type {
		case "index":
			indexes[name] = struct{}{}
		case "trigger":
			triggers[name] = struct{}{}
		case "view":
			views[name] = struct{}{}
		case "table":
			tables[name] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return errors.Trace(err)
	}

	// Do not attempt to delete the database all in one query. Dqlite can't
	// handle it and we end up causing a segfault. Instead we'll create batches
	// of queries that we can execute in a single transaction, but over multiple
	// queries.

	// Ensure that we remove everything in the correct order.
	// 1. Drop all the contents of the tables.
	// 2. Remove all indexes.
	// 3. Remove all triggers.
	// 4. Drop all views.
	// 5. Drop all tables (ignoring sqlite_master)
	var stmts []string
	for name := range tables {
		stmts = append(stmts, fmt.Sprintf("DELETE FROM %q;", name))
	}
	for name := range indexes {
		stmts = append(stmts, fmt.Sprintf("DROP INDEX IF EXISTS %q;", name))
	}
	for name := range triggers {
		stmts = append(stmts, fmt.Sprintf("DROP TRIGGER IF EXISTS %q;", name))
	}
	for name := range views {
		stmts = append(stmts, fmt.Sprintf("DROP VIEW IF EXISTS %q;", name))
	}
	for name := range tables {
		stmts = append(stmts, fmt.Sprintf("DROP TABLE IF EXISTS %q;", name))
	}

	logger.Debugf(ctx, "deleting database contents: %d statements", len(stmts))

	// Batch the statements into groups, so we don't exceed the maximum
	// number of statements in a single transaction.
	const maxBatchSize = 15
	for i := 0; i < len(stmts); i += maxBatchSize {
		batch := stmts[i:min(i+maxBatchSize, len(stmts))]
		logger.Debugf(ctx, "executing batch %d with %d statements", i, len(batch))

		if _, err := tx.ExecContext(ctx, strings.Join(batch, "\n")); err != nil {
			return errors.Trace(err)
		}
	}

	logger.Infof(ctx, "database contents deleted")

	return nil
}
