// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:generate go run .

package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/canonical/sqlair"
	_ "github.com/mattn/go-sqlite3"

	"github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/export"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/logger"
)

type txnRunner struct{ db *sql.DB }

func (r *txnRunner) Txn(ctx context.Context, f func(context.Context, *sqlair.TX) error) error {
	return database.Txn(ctx, sqlair.NewDB(r.db), f)
}

func (r *txnRunner) StdTxn(ctx context.Context, f func(context.Context, *sql.Tx) error) error {
	return database.StdTxn(ctx, r.db, f)
}

func (r *txnRunner) Dying() <-chan struct{} { return nil }

type column struct {
	Name, Field                  string
	Binary, BinaryText, Nullable bool
}

type table struct {
	Name, Struct, Insert string
	Columns              []column
	Exclusion            string
	HasBinary            bool
}

type pass struct {
	Kind, Prefix, Version, ExportPath string
	Tables                            []table
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	for _, kind := range []string{"controller", "model"} {
		if err := generate(ctx, kind); err != nil {
			return fmt.Errorf("generating %s recovery: %w", kind, err)
		}
	}
	return nil
}

func generate(ctx context.Context, kind string) error {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return err
	}
	defer db.Close()
	runner := &txnRunner{db: db}
	ddl := schema.ModelDDLForVersion(version.Current)
	p := pass{Kind: kind, Prefix: "Model", Version: export.LatestSupportedPayloadVersion().String()}
	if kind == "controller" {
		ddl = schema.ControllerDDLForVersion(version.Current)
		p.Prefix = "Controller"
		p.Version = export.LatestControllerExportVersion().String()
	}
	p.ExportPath = "github.com/juju/juju/domain/export/types/"
	if kind == "controller" {
		p.ExportPath += "controller/"
	}
	p.ExportPath += "v" + strings.ReplaceAll(p.Version, ".", "_")
	if err := database.NewDBMigration(runner, logger.Noop(), ddl).Apply(ctx); err != nil {
		return err
	}
	p.Tables, err = inspect(ctx, db)
	if err != nil {
		return err
	}
	for i := range p.Tables {
		p.Tables[i].Exclusion = recoveryExclusions[p.Tables[i].Name]
	}
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	root := filepath.Dir(filepath.Dir(dir))
	for _, out := range []struct{ template, destination string }{
		{"types.tmpl", filepath.Join(root, "domain/recovery", kind+"_gen.go")},
		{"decode.tmpl", filepath.Join(root, "domain/recovery/service", kind+"_decode_gen.go")},
		{"load.tmpl", filepath.Join(root, "domain/recovery/state", kind+"_gen.go")},
	} {
		if err := render(filepath.Join(dir, out.template), out.destination, p); err != nil {
			return err
		}
	}
	return nil
}

func inspect(ctx context.Context, db *sql.DB) ([]table, error) {
	rows, err := db.QueryContext(ctx, `SELECT sm.name FROM sqlite_master AS sm
WHERE sm.type = 'table' AND sm.name NOT LIKE 'sqlite_%' ORDER BY sm.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var tables []table
	for _, name := range names {
		t, err := inspectTable(ctx, db, name)
		if err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, nil
}

func inspectTable(ctx context.Context, db *sql.DB, name string) (table, error) {
	t := table{Name: name, Struct: camel(name)}
	cols, err := db.QueryContext(ctx, `PRAGMA table_info("`+name+`")`)
	if err != nil {
		return table{}, err
	}
	defer cols.Close()
	for cols.Next() {
		var id, notNull, primaryKey int
		var name, typ string
		var def sql.NullString
		if err := cols.Scan(&id, &name, &typ, &notNull, &def, &primaryKey); err != nil {
			return table{}, err
		}
		storageType := schema.ColumnStorageType(t.Name, name, typ)
		binary := storageType == "BLOB"
		t.Columns = append(t.Columns, column{
			Name:       name,
			Field:      camel(name),
			Binary:     binary,
			BinaryText: binary && strings.EqualFold(typ, "TEXT"),
			Nullable:   notNull == 0,
		})
		t.HasBinary = t.HasBinary || binary
	}
	if err := cols.Err(); err != nil {
		return table{}, err
	}
	if err := cols.Close(); err != nil {
		return table{}, err
	}
	t.Insert = insertSQL(t)
	return t, nil
}

func insertSQL(t table) string {
	var names, values []string
	for _, col := range t.Columns {
		names = append(names, `"`+col.Name+`"`)
		binding := t.Struct
		if t.HasBinary {
			binding = "M"
		}
		value := "$" + binding + "." + col.Name
		values = append(values, value+` AS "`+col.Name+`"`)
	}
	return `INSERT INTO "` + t.Name + `" (` + strings.Join(names, ", ") + `) SELECT ` + strings.Join(values, ", ")
}

func camel(s string) string {
	var result strings.Builder
	for part := range strings.SplitSeq(s, "_") {
		switch part {
		case "uuid", "id":
			result.WriteString(strings.ToUpper(part))
		default:
			result.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return result.String()
}

func render(source, dest string, data pass) error {
	t, err := template.ParseFiles(source)
	if err != nil {
		return err
	}
	var output bytes.Buffer
	if err := t.Execute(&output, data); err != nil {
		return err
	}
	formatted, err := format.Source(output.Bytes())
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(dest); err == nil && bytes.Equal(existing, formatted) {
		return nil
	}
	return os.WriteFile(dest, formatted, 0o644)
}
