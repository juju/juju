// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/juju/tc"
	"github.com/mattn/go-sqlite3"

	coreschema "github.com/juju/juju/core/database/schema"
	coretesting "github.com/juju/juju/internal/testing"
)

type dependencyOrderSuite struct {
	coretesting.BaseSuite
}

func TestDependencyOrderSuite(t *testing.T) {
	tc.Run(t, &dependencyOrderSuite{})
}

func (s *dependencyOrderSuite) TestSchemaOrder(c *tc.C) {
	for _, entry := range []struct {
		name string
		ddl  *coreschema.Schema
	}{
		{"controller", ControllerDDL()},
		{"model", ModelDDL()},
	} {
		var statements []string
		for _, patch := range entry.ddl.Patches() {
			statements = append(statements, coreschema.Stmt(patch))
		}
		err := checkDependencyOrder(c.Context(), statements)
		c.Check(err, tc.ErrorIsNil, tc.Commentf("%s", entry.name))
	}
}

func (s *dependencyOrderSuite) TestDependencyOrder(c *tc.C) {
	for _, test := range []struct {
		name       string
		statements []string
		wantError  string
	}{
		{
			name: "prior composite key and self reference",
			statements: []string{`
CREATE TABLE parent (a TEXT, b TEXT);
CREATE UNIQUE INDEX parent_key ON parent (a, b);
CREATE TABLE child (a TEXT, b TEXT, FOREIGN KEY (a, b) REFERENCES parent (a, b));
CREATE TABLE self (id TEXT PRIMARY KEY, parent TEXT REFERENCES self (id));
CREATE VIEW child_view AS SELECT a, b FROM child;`,
			},
		},
		{
			name:       "forward table in same patch",
			statements: []string{`CREATE TABLE child (id TEXT REFERENCES parent (id)); CREATE TABLE parent (id TEXT PRIMARY KEY);`},
			wantError:  `validating table "child": no such table: main.parent`,
		},
		{
			name: "forward unique index",
			statements: []string{`
CREATE TABLE parent (a TEXT, b TEXT);
CREATE TABLE child (a TEXT, b TEXT, FOREIGN KEY (a, b) REFERENCES parent (a, b));
CREATE UNIQUE INDEX parent_key ON parent (a, b);`,
			},
			wantError: `validating table "child": foreign key mismatch.*`,
		},
		{
			name: "earlier key has the wrong collation",
			statements: []string{`
CREATE TABLE parent (id TEXT);
CREATE UNIQUE INDEX wrong_key ON parent (id COLLATE NOCASE);
CREATE TABLE child (id TEXT REFERENCES parent (id));
CREATE UNIQUE INDEX valid_key ON parent (id);`,
			},
			wantError: `validating table "child": foreign key mismatch.*`,
		},
		{
			name:       "forward view source in same patch",
			statements: []string{`CREATE VIEW v AS SELECT id FROM parent; CREATE TABLE parent (id TEXT);`},
			wantError:  `view "v" refers to later object "parent"`,
		},
		{
			name: "forward trigger source in same patch",
			statements: []string{`
CREATE TABLE child (id TEXT);
CREATE TRIGGER child_insert AFTER INSERT ON child BEGIN INSERT INTO parent VALUES (NEW.id); END;
CREATE TABLE parent (id TEXT);`,
			},
			wantError: `trigger "child_insert" refers to later object "parent"`,
		},
		{
			name: "forward source in an update-of trigger",
			statements: []string{`
CREATE TABLE child (id TEXT, value TEXT);
CREATE TRIGGER child_update AFTER UPDATE OF value ON child BEGIN INSERT INTO parent VALUES (NEW.id); END;
CREATE TABLE parent (id TEXT);`,
			},
			wantError: `trigger "child_update" refers to later object "parent"`,
		},
	} {
		err := checkDependencyOrder(c.Context(), test.statements)
		if test.wantError == "" {
			c.Check(err, tc.ErrorIsNil, tc.Commentf("%s", test.name))
		} else {
			c.Check(err, tc.ErrorMatches, test.wantError, tc.Commentf("%s", test.name))
		}
	}
}

type schemaObject struct {
	name, kind, statement string
	order                 int64
}

type schemaTrigger struct {
	name, table, statement string
	order                  int64
}

// checkDependencyOrder uses SQLite's own schema metadata and view resolver,
// rather than searching SQL text, to check ordering within and across patches.
func checkDependencyOrder(ctx context.Context, statements []string) error {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("applying schema: %w", err)
		}
	}

	rows, err := db.QueryContext(ctx, `SELECT name, type, rowid, tbl_name, coalesce(sql, '') FROM sqlite_master WHERE type IN ('table', 'view', 'index', 'trigger') ORDER BY rowid`)
	if err != nil {
		return err
	}
	defer rows.Close()
	objects := make(map[string]schemaObject)
	var orderedObjects []schemaObject
	var views []string
	var triggers []schemaTrigger
	for rows.Next() {
		var name, kind, table, statement string
		var order int64
		if err := rows.Scan(&name, &kind, &order, &table, &statement); err != nil {
			rows.Close()
			return err
		}
		object := schemaObject{name: name, kind: kind, statement: statement, order: order}
		objects[name] = object
		orderedObjects = append(orderedObjects, object)
		switch kind {
		case "view":
			views = append(views, name)
		case "trigger":
			triggers = append(triggers, schemaTrigger{name: name, table: table, statement: statement, order: order})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	if err := checkForeignKeysCompile(ctx, orderedObjects); err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	for _, view := range views {
		if err := checkViewReferences(conn, objects, view); err != nil {
			return err
		}
	}
	if err := checkTriggerReferences(ctx, conn, objects, triggers); err != nil {
		return err
	}
	return nil
}

func checkForeignKeysCompile(ctx context.Context, objects []schemaObject) error {
	db, err := sql.Open("sqlite3", "file::memory:?_foreign_keys=1")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, object := range objects {
		if strings.HasPrefix(object.name, "sqlite_") ||
			(object.kind != "table" && object.kind != "index") {
			continue
		}
		if object.statement == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, object.statement); err != nil {
			return fmt.Errorf("creating %s %q: %w", object.kind, object.name, err)
		}
		if object.kind != "table" {
			continue
		}
		if err := checkTableKey(ctx, db, object.name); err != nil {
			return fmt.Errorf("validating table %q: %w", object.name, err)
		}
	}
	return nil
}

func checkTableKey(ctx context.Context, db *sql.DB, table string) error {
	// Preparing the insert validates foreign keys against the schema so far;
	// EXPLAIN avoids writing data.
	stmt, err := db.PrepareContext(ctx, `EXPLAIN INSERT INTO `+quoteIdentifier(table)+` DEFAULT VALUES`)
	if err != nil {
		return err
	}
	return stmt.Close() //nolint:sqlclosecheck // Close immediately and return its error.
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func checkTriggerReferences(ctx context.Context, conn *sql.Conn, objects map[string]schemaObject, triggers []schemaTrigger) error {
	for _, trigger := range triggers {
		if _, err := conn.ExecContext(ctx, `DROP TRIGGER `+quoteIdentifier(trigger.name)); err != nil {
			return err
		}
	}
	for _, trigger := range triggers {
		if _, err := conn.ExecContext(ctx, trigger.statement); err != nil {
			return fmt.Errorf("creating trigger %q: %w", trigger.name, err)
		}
		columns, err := triggerColumns(ctx, conn, trigger.table)
		if err != nil {
			return fmt.Errorf("reading trigger %q target: %w", trigger.name, err)
		}
		var assignments []string
		for _, column := range columns {
			assignments = append(assignments, quoteIdentifier(column)+` = `+quoteIdentifier(column))
		}
		table := quoteIdentifier(trigger.table)
		queries := []string{
			`EXPLAIN INSERT INTO ` + table + ` DEFAULT VALUES`,
			`EXPLAIN UPDATE ` + table + ` SET ` + strings.Join(assignments, ", "),
			`EXPLAIN DELETE FROM ` + table,
		}
		err = conn.Raw(func(raw any) error {
			sqlite := raw.(*sqlite3.SQLiteConn)
			dependencies := make(map[string]bool)
			sqlite.RegisterAuthorizer(func(op int, first, second, database string) int {
				if op == sqlite3.SQLITE_READ || op == sqlite3.SQLITE_INSERT ||
					op == sqlite3.SQLITE_UPDATE || op == sqlite3.SQLITE_DELETE {
					dependencies[first] = true
				}
				return sqlite3.SQLITE_OK
			})
			defer sqlite.RegisterAuthorizer(nil)
			for _, query := range queries {
				stmt, err := sqlite.Prepare(query)
				if err != nil {
					return fmt.Errorf("preparing trigger %q: %w", trigger.name, err)
				}
				if err := stmt.Close(); err != nil {
					return err
				}
			}
			var names []string
			for name := range dependencies {
				names = append(names, name)
			}
			slices.Sort(names)
			for _, name := range names {
				if strings.HasPrefix(name, "sqlite_") {
					continue
				}
				object, exists := objects[name]
				if !exists {
					return fmt.Errorf("trigger %q refers to missing object %q", trigger.name, name)
				}
				if object.order > trigger.order {
					return fmt.Errorf("trigger %q refers to later object %q", trigger.name, name)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `DROP TRIGGER `+quoteIdentifier(trigger.name)); err != nil {
			return err
		}
	}
	return nil
}

func triggerColumns(ctx context.Context, conn *sql.Conn, table string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("trigger target %q has no columns", table)
	}
	return columns, nil
}

func checkViewReferences(conn *sql.Conn, objects map[string]schemaObject, view string) error {
	return conn.Raw(func(raw any) error {
		sqlite := raw.(*sqlite3.SQLiteConn)
		dependencies := make(map[string]bool)
		sqlite.RegisterAuthorizer(func(op int, table, column, database string) int {
			if op == sqlite3.SQLITE_READ && table != view {
				dependencies[table] = true
			}
			return sqlite3.SQLITE_OK
		})
		defer sqlite.RegisterAuthorizer(nil)
		stmt, err := sqlite.Prepare(`SELECT * FROM "` + strings.ReplaceAll(view, `"`, `""`) + `" LIMIT 0`)
		if err != nil {
			return fmt.Errorf("preparing view %q: %w", view, err)
		}
		if err := stmt.Close(); err != nil {
			return err
		}
		var names []string
		for name := range dependencies {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			object, exists := objects[name]
			if !exists {
				return fmt.Errorf("view %q refers to missing object %q", view, name)
			}
			if object.order > objects[view].order {
				return fmt.Errorf("view %q refers to later object %q", view, name)
			}
		}
		return nil
	})
}
