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
	Name, Field, Type, Default string
	HasDefault                 bool
}

type table struct {
	Name, Struct, Insert string
	Columns              []column
}

type pass struct {
	Kind, Prefix, Version string
	Tables                []table
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
	if err := database.NewDBMigration(runner, logger.Noop(), ddl).Apply(ctx); err != nil {
		return err
	}
	p.Tables, err = inspect(ctx, db, p.Prefix)
	if err != nil {
		return err
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

func inspect(ctx context.Context, db *sql.DB, prefix string) ([]table, error) {
	rows, err := db.QueryContext(ctx, `SELECT sm.name FROM sqlite_master AS sm
WHERE sm.type = 'table' AND sm.name NOT LIKE 'sqlite_%' ORDER BY sm.name`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var tables []table
	for _, name := range names {
		t := table{Name: name, Struct: prefix + camel(name)}
		cols, err := db.QueryContext(ctx, `PRAGMA table_info("`+name+`")`)
		if err != nil {
			return nil, err
		}
		for cols.Next() {
			var id, notNull, primaryKey int
			var name, typ string
			var def sql.NullString
			if err := cols.Scan(&id, &name, &typ, &notNull, &def, &primaryKey); err != nil {
				_ = cols.Close()
				return nil, err
			}
			t.Columns = append(t.Columns, column{
				Name: name, Field: camel(name), Type: bindingType(t.Name, name, typ),
				HasDefault: def.Valid, Default: def.String,
			})
		}
		if err := cols.Err(); err != nil {
			_ = cols.Close()
			return nil, err
		}
		if err := cols.Close(); err != nil {
			return nil, err
		}
		t.Insert = insertSQL(t)
		tables = append(tables, t)
	}
	return tables, nil
}

// binaryTextColumns describes legacy TEXT columns whose domain writers bind
// bytes. Their export structs use strings, so the archive can contain either
// ordinary UTF-8 strings or !!binary values. Recover both as BLOBs, as the
// original writers do. Other TEXT columns remain string bindings.
var binaryTextColumns = map[string][]string{
	"bakery_config": {
		"local_users_private_key", "local_users_public_key",
		"local_users_third_party_private_key", "local_users_third_party_public_key",
		"external_users_third_party_private_key", "external_users_third_party_public_key",
		"offers_third_party_private_key", "offers_third_party_public_key",
	},
	"macaroon_root_key":                            {"id", "root_key"},
	"user_password":                                {"password_salt"},
	"user_activation_key":                          {"activation_key"},
	"charm":                                        {"lxd_profile"},
	"charm_metadata":                               {"assumes"},
	"charm_action":                                 {"params"},
	"machine_status":                               {"data"},
	"machine_cloud_instance_status":                {"data"},
	"operator_status":                              {"data"},
	"application_status":                           {"data"},
	"unit_agent_status":                            {"data"},
	"unit_workload_status":                         {"data"},
	"k8s_pod_status":                               {"data"},
	"application_remote_offerer":                   {"macaroon"},
	"application_remote_offerer_status":            {"data"},
	"application_remote_offerer_relation_macaroon": {"macaroon"},
}

func bindingType(table, name, typ string) string {
	for _, binaryColumn := range binaryTextColumns[table] {
		if name == binaryColumn {
			return "[]byte"
		}
	}
	switch strings.ToUpper(typ) {
	case "INTEGER", "INT":
		return "int64"
	case "BOOLEAN":
		return "bool"
	case "DATETIME", "TIMESTAMP", "DATE":
		return "time.Time"
	case "BLOB":
		return "[]byte"
	case "REAL", "FLOAT", "DOUBLE":
		return "float64"
	case "TEXT":
		return "string"
	default:
		return "any"
	}
}

func insertSQL(t table) string {
	var names, values []string
	for _, col := range t.Columns {
		names = append(names, `"`+col.Name+`"`)
		value := "$" + t.Struct + "." + col.Name
		if col.HasDefault {
			value = "CASE WHEN $" + t.Struct + ".has_" + col.Name + " THEN " + value + " ELSE " + col.Default + " END"
		}
		values = append(values, value+` AS "`+col.Name+`"`)
	}
	return `INSERT INTO "` + t.Name + `" (` + strings.Join(names, ", ") + `) SELECT ` + strings.Join(values, ", ")
}

func camel(s string) string {
	var result strings.Builder
	for _, part := range strings.Split(s, "_") {
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
