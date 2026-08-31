// Copyright 2025 Canonical Ltd.
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
	"slices"
	"sort"
	"strings"
	"text/template"

	"github.com/canonical/sqlair"
	_ "github.com/mattn/go-sqlite3"

	coreschema "github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/export"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/logger"
)

// txnRunner is the simplest possible implementation of
// [core.database.TxnRunner]. It is used here to run database
// migrations and query schema metadata.
type txnRunner struct {
	db *sql.DB
}

func (r *txnRunner) Txn(ctx context.Context, f func(context.Context, *sqlair.TX) error) error {
	return database.Txn(ctx, sqlair.NewDB(r.db), f)
}

func (r *txnRunner) StdTxn(ctx context.Context, f func(context.Context, *sql.Tx) error) error {
	return database.StdTxn(ctx, r.db, f)
}

func (r *txnRunner) Dying() <-chan struct{} {
	return nil
}

func main() {
	fmt.Printf("Juju version: %s\n", version.Current)

	ctx := context.Background()
	for _, pass := range []generatorPass{modelPass(), controllerPass()} {
		if err := runPass(ctx, pass); err != nil {
			fmt.Fprintf(os.Stderr, "failed to generate %s export: %v\n", pass.kind, err)
			os.Exit(1)
		}
	}
}

// generatedFile pairs a template with the file it renders into. Template paths
// are relative to this generator's directory, output directories to the
// repository root.
type generatedFile struct {
	template string
	dir      string
	name     string
}

// generatorPass describes one pass of the generator over a database schema:
// the schema to introspect, the export versions it is generated for, the tables
// it never exports, and the files it emits.
type generatorPass struct {
	// kind names the schema in progress and error messages.
	kind string

	// ddl is the schema applied to the scratch database the pass introspects.
	ddl *coreschema.Schema

	// versions are the export schema versions of this pass. The highest one is
	// the version generated.
	versions []semversion.Number

	// excludedTables are the tables the pass must never export. Beyond
	// SQLite's own bookkeeping this is how node-local and unbounded tables are
	// kept out of the payload.
	excludedTables []string

	// types is the payload types package. It is versioned, so the version
	// directory (v<token>) is appended to its dir.
	types generatedFile

	// state and stateTest are the state layer running the export queries.
	state, stateTest generatedFile

	// service and serviceTest are the service layer wrapping the state.
	service, serviceTest generatedFile

	// postGenerate runs once the files above are written. Only the model pass
	// uses it, to emit the version-to-version transforms.
	postGenerate func(versions []semversion.Number) error
}

// modelPass generates the model-schema export: payload types per supported
// export version, plus the transforms walking a payload from an older version
// up to the latest.
func modelPass() generatorPass {
	return generatorPass{
		kind:     "model",
		ddl:      schema.ModelDDLForVersion(version.Current),
		versions: export.ExportVersions,
		// sqlite_sequence is SQLite's own AUTOINCREMENT bookkeeping, not model
		// data.
		excludedTables: []string{"sqlite_sequence"},
		types:          generatedFile{template: "types.tmpl", dir: "domain/export/types", name: "model.go"},
		state:          generatedFile{template: "state.tmpl", dir: "domain/export/state/model", name: "export.go"},
		stateTest:      generatedFile{template: "state_test.tmpl", dir: "domain/export/state/model", name: "export_test.go"},
		service:        generatedFile{template: "service.tmpl", dir: "domain/export/service", name: "export.go"},
		serviceTest:    generatedFile{template: "service_test.tmpl", dir: "domain/export/service", name: "export_test.go"},
		postGenerate: func(versions []semversion.Number) error {
			return generateTransforms(exportVersionStrings(versions))
		},
	}
}

// controllerPass generates the controller-schema export. Transforms stay
// model-only: the controller payload has no version history and its only
// consumer is the backup feature.
func controllerPass() generatorPass {
	return generatorPass{
		kind:     "controller",
		ddl:      schema.ControllerDDLForVersion(version.Current),
		versions: export.ControllerExportVersions,
		// A controller backup is a faithful snapshot: every table the schema
		// defines is exported, and deciding what a restore replays is the
		// restore path's business, not the export's. Only SQLite's own
		// AUTOINCREMENT bookkeeping is skipped.
		excludedTables: []string{"sqlite_sequence"},
		types:          generatedFile{template: "controller_types.tmpl", dir: "domain/export/types/controller", name: "controller.go"},
		state:          generatedFile{template: "controller_state.tmpl", dir: "domain/export/state/controller", name: "export.go"},
		stateTest:      generatedFile{template: "controller_state_test.tmpl", dir: "domain/export/state/controller", name: "export_test.go"},
		service:        generatedFile{template: "controller_service.tmpl", dir: "domain/export/service", name: "controller_export.go"},
		serviceTest:    generatedFile{template: "controller_service_test.tmpl", dir: "domain/export/service", name: "controller_export_test.go"},
	}
}

// runPass applies the pass's schema to a scratch in-memory database and
// generates the pass's files from it.
func runPass(ctx context.Context, pass generatorPass) error {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	runner := &txnRunner{db: db}
	if err := database.NewDBMigration(runner, logger.Noop(), pass.ddl).Apply(ctx); err != nil {
		return fmt.Errorf("applying schema: %w", err)
	}
	fmt.Printf("Applied %s schema.\n", pass.kind)

	return generate(ctx, runner, pass)
}

// templateData carries every value the generator templates consume; each
// template uses the subset it needs.
type templateData struct {
	// VersionToken is the export version with dots replaced by underscores, for
	// use in package names, directory names and Go identifiers.
	VersionToken string

	// SemanticVersion is the export version in its dotted form.
	SemanticVersion string

	// Imports are the packages the generated row structs need, sorted.
	Imports []string

	// TableNames are the exported tables, sorted and parallel to StructNames.
	TableNames []string

	// Structs are the generated row struct definitions.
	Structs []string

	// StructNames are the row struct names, parallel to TableNames.
	StructNames []string
}

// generate introspects the pass's schema, builds a row struct per exported
// table, and renders the pass's types, state and service files.
func generate(ctx context.Context, runner *txnRunner, pass generatorPass) error {
	if len(pass.versions) == 0 {
		return fmt.Errorf("no %s export versions defined", pass.kind)
	}
	semanticVersion := slices.MaxFunc(pass.versions, semversion.Number.Compare).String()

	tableNames, err := getTableNames(ctx, runner)
	if err != nil {
		return err
	}

	data := templateData{
		// Transform dots to underscores for use in package and directory names.
		VersionToken:    strings.ReplaceAll(semanticVersion, ".", "_"),
		SemanticVersion: semanticVersion,
	}

	imports := make(map[string]struct{})
	for _, tableName := range tableNames {
		if slices.Contains(pass.excludedTables, tableName) {
			continue
		}

		columns, err := getTableSchema(ctx, runner, tableName)
		if err != nil {
			return err
		}

		structDef, requiredImports, err := generateStruct(tableName, columns)
		if err != nil {
			return err
		}

		data.Structs = append(data.Structs, structDef)
		data.StructNames = append(data.StructNames, toCamelCase(tableName))
		data.TableNames = append(data.TableNames, tableName)
		for _, imp := range requiredImports {
			imports[imp] = struct{}{}
		}
	}
	data.Imports = sortedImports(imports)

	// The payload types are versioned: every supported export version keeps its
	// own package alongside the others.
	typesFile := pass.types
	typesFile.dir = filepath.Join(typesFile.dir, fmt.Sprintf("v%s", data.VersionToken))

	for _, file := range []generatedFile{
		typesFile, pass.state, pass.stateTest, pass.service, pass.serviceTest,
	} {
		if err := renderFile(file, data); err != nil {
			return err
		}
	}

	if pass.postGenerate == nil {
		return nil
	}
	return pass.postGenerate(pass.versions)
}

// renderFile executes one template against data and writes the gofmt-ed
// result. A formatting failure is reported rather than worked around: it means
// the template emitted code that does not parse, and writing the unformatted
// bytes anyway would bake that into a checked-in generated file.
func renderFile(file generatedFile, data templateData) error {
	tmplBytes, err := os.ReadFile(filepath.Join(generatorDir(), file.template))
	if err != nil {
		return err
	}

	t, err := template.New(file.template).Parse(string(tmplBytes))
	if err != nil {
		return fmt.Errorf("parsing %s: %w", file.template, err)
	}

	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return fmt.Errorf("executing %s: %w", file.template, err)
	}

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return fmt.Errorf("formatting output of %s: %w", file.template, err)
	}

	dir := filepath.Join(repoRoot(), file.dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(dir, file.name)
	fmt.Printf("writing to %s\n", filePath)
	return os.WriteFile(filePath, formatted, 0644)
}

// sortedImports flattens the collected import set into a sorted slice, so that
// regenerating an unchanged schema produces an unchanged file.
func sortedImports(imports map[string]struct{}) []string {
	sorted := make([]string, 0, len(imports))
	for imp := range imports {
		sorted = append(sorted, imp)
	}
	sort.Strings(sorted)
	return sorted
}

func exportVersionStrings(versions []semversion.Number) []string {
	result := make([]string, len(versions))
	for i, v := range versions {
		result[i] = v.String()
	}
	return result
}

func getTableNames(ctx context.Context, runner *txnRunner) ([]string, error) {
	var tableNames []string
	err := runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		tableNames = nil

		rows, err := tx.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table'")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return err
			}
			tableNames = append(tableNames, name)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(tableNames)
	return tableNames, nil
}

type column struct {
	Name    string
	Type    string
	NotNull bool
}

func getTableSchema(ctx context.Context, runner *txnRunner, tableName string) ([]column, error) {
	var columns []column
	query := fmt.Sprintf("PRAGMA table_info(%q)", tableName)
	err := runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		columns = nil

		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var cid int
			var name, typ, defaultVal sql.NullString
			var notnull, pk int
			if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultVal, &pk); err != nil {
				return err
			}
			columns = append(columns, column{
				Name:    name.String,
				Type:    typ.String,
				NotNull: notnull != 0,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return columns, nil
}

// toCamelCase converts snake case identifiers from the database to
// camel case identifiers for Go types.
// Exceptions are made for "id" and "uuid", which become all caps.
func toCamelCase(s string) string {
	if s == "" {
		return ""
	}

	parts := strings.Split(s, "_")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		switch strings.ToLower(p) {
		case "id":
			b.WriteString("ID")
		case "uuid":
			b.WriteString("UUID")
		default:
			l := strings.ToLower(p)
			b.WriteString(strings.ToUpper(l[:1]) + l[1:])
		}
	}

	return b.String()
}

func generateStruct(tableName string, columns []column) (string, []string, error) {
	structName := toCamelCase(tableName)
	var sb strings.Builder

	if _, err := sb.WriteString(fmt.Sprintf("type %s struct {\n", structName)); err != nil {
		return "", nil, err
	}

	var imports []string
	for _, col := range columns {
		goType, imp := sqliteTypeToGoType(col.Type, col.NotNull)
		if imp != "" {
			imports = append(imports, imp)
		}
		fieldName := toCamelCase(col.Name)

		if _, err := sb.WriteString(
			fmt.Sprintf(
				"\t%s %s `db:%q json:%q yaml:%q`\n",
				fieldName,
				goType,
				col.Name,
				col.Name,
				col.Name,
			),
		); err != nil {
			return "", nil, err
		}
	}

	if _, err := sb.WriteString("}\n"); err != nil {
		return "", nil, err
	}

	return sb.String(), imports, nil
}

func sqliteTypeToGoType(sqliteType string, notNull bool) (string, string) {
	var goType, imp string

	switch strings.ToUpper(sqliteType) {
	case "INTEGER", "INT":
		goType = "int64"
	case "TEXT":
		goType = "string"
	case "BOOLEAN":
		goType = "bool"
	case "DATETIME", "TIMESTAMP":
		goType = "time.Time"
		imp = "time"
	case "BLOB":
		goType = "[]byte"
	default:
		goType = "any"
	}

	if !notNull {
		goType = "*" + goType
	}
	return goType, imp
}
