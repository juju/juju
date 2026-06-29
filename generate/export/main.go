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
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"text/template"

	"github.com/canonical/sqlair"
	_ "github.com/mattn/go-sqlite3"

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

// bootstrapTables are the target-local identity/agent tables the target creates
// itself when it bootstraps the model DB during a v8 import (see
// internal/migration/v2). They must never be inserted from the source payload,
// so the generated importer excludes them.
var bootstrapTables = map[string]bool{
	"model":           true, // read-only model identity (0004-read-only-model.sql)
	"model_life":      true,
	"agent_version":   true,
	"model_agent":     true,
	"model_migrating": true,
}

// nonContentTables are operational/changestream tables that are not part of a
// model's migratable content. The target's changestream starts fresh, so these
// are excluded from the generated importer. The seeded ones among them
// (change_log_edit_type, change_log_namespace) are also auto-excluded by
// getSeededTables; they are listed here for clarity.
//
// NOTE: this list (and bootstrapTables) is the import-exclusion contract.
//
// TODO audit it for other non-payload tables — e.g. binary/object-store tables
// whose blobs transfer over separate HTTP endpoints rather than in the YAML
// payload.
var nonContentTables = map[string]bool{
	"change_log":           true,
	"change_log_witness":   true,
	"change_log_edit_type": true,
	"change_log_namespace": true,
}

func main() {
	fmt.Printf("Juju version: %s\n", version.Current)

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	runner := &txnRunner{db: db}
	m := database.NewDBMigration(runner, logger.Noop(), schema.ModelDDLForVersion(version.Current))

	ctx := context.Background()
	if err := m.Apply(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to apply migration: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Applied model schema.")

	if err := generate(ctx, runner); err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate schema: %v\n", err)
		os.Exit(1)
	}
}

func generate(ctx context.Context, runner *txnRunner) error {
	if len(export.ExportVersions) == 0 {
		return fmt.Errorf("no export versions defined")
	}
	semanticVersion := slices.MaxFunc(export.ExportVersions, semversion.Number.Compare).String()

	// Transform dots to underscores for use in package and directory names.
	versionToken := strings.ReplaceAll(semanticVersion, ".", "_")

	tableNames, err := getTableNames(ctx, runner)
	if err != nil {
		return err
	}

	var structs, structNames, usedTableNames []string
	imports := make(map[string]struct{})

	for _, tableName := range tableNames {
		if tableName == "sqlite_sequence" {
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

		structs = append(structs, structDef)
		structNames = append(structNames, toCamelCase(tableName))
		usedTableNames = append(usedTableNames, tableName)
		for _, imp := range requiredImports {
			imports[imp] = struct{}{}
		}
	}

	if err := writeTypesFile(versionToken, usedTableNames, structs, structNames, imports); err != nil {
		return err
	}

	if err := writeStateModelVersionFile(versionToken, semanticVersion, usedTableNames, structNames); err != nil {
		return err
	}

	if err := writeServiceModelVersionFile(versionToken, semanticVersion); err != nil {
		return err
	}

	// The importer is the write-mirror of the exporter. It covers every table the
	// exporter does, minus the target-local bootstrap tables and the
	// operational/changestream tables. Tables the schema seeds (lookup tables, and
	// seeded-extensible tables such as space which has the well-known alpha row plus
	// user rows) are kept but inserted with ON CONFLICT DO NOTHING: their identical
	// seed rows are skipped while genuine content rows are inserted. usedTableNames
	// and structNames are parallel.
	seeded, err := getSeededTables(ctx, runner, usedTableNames)
	if err != nil {
		return err
	}
	var importTables []importTableData
	for i, tableName := range usedTableNames {
		if bootstrapTables[tableName] || nonContentTables[tableName] {
			continue
		}
		importTables = append(importTables, importTableData{
			StructName: structNames[i],
			TableName:  tableName,
			Seeded:     seeded[tableName],
		})
	}

	if err := writeStateImportFile(versionToken, semanticVersion, importTables); err != nil {
		return err
	}

	return generateTransforms(exportVersionStrings(export.ExportVersions))
}

// getSeededTables returns the set of tables that already contain rows after the
// schema has been applied to the empty in-memory DB. These are the lookup tables
// the schema seeds (life, secret_role, ip_address_type, ...). The target seeds
// the same rows, so the importer must not re-insert them (it would collide on the
// primary key).
func getSeededTables(ctx context.Context, runner *txnRunner, tableNames []string) (map[string]bool, error) {
	seeded := make(map[string]bool)
	err := runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, tableName := range tableNames {
			var count int
			query := fmt.Sprintf("SELECT COUNT(*) FROM %q", tableName)
			if err := tx.QueryRowContext(ctx, query).Scan(&count); err != nil {
				return err
			}
			if count > 0 {
				seeded[tableName] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return seeded, nil
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

func writeTypesFile(
	version string,
	tableNames []string,
	structs []string,
	structNames []string,
	imports map[string]struct{},
) error {
	// We should be in domain/export/generate.
	_, filename, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(filename)

	// Target directory is always under the repository's domain/export path.
	repoRoot := filepath.Dir(filepath.Dir(currentDir)) // generate/export -> generate -> repo root
	dir := filepath.Join(repoRoot, "domain", "export", "types", fmt.Sprintf("v%s", version))

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Prepare import slice sorted for stable output.
	sortedImports := make([]string, 0, len(imports))
	for imp := range imports {
		sortedImports = append(sortedImports, imp)
	}
	sort.Strings(sortedImports)

	tmplPath := filepath.Join(filepath.Dir(filename), "types.tmpl")
	tmplBytes, err := os.ReadFile(tmplPath)
	if err != nil {
		return err
	}

	data := struct {
		Version     string
		Imports     []string
		TableNames  []string
		Structs     []string
		StructNames []string
	}{
		Version:     version,
		Imports:     sortedImports,
		TableNames:  tableNames,
		Structs:     structs,
		StructNames: structNames,
	}

	t := template.Must(template.New("types").Parse(string(tmplBytes)))
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return err
	}

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return err
	}

	filePath := filepath.Join(dir, "model.go")
	fmt.Printf("writing to %s\n", filePath)
	return os.WriteFile(filePath, formatted, 0644)
}

func writeStateModelVersionFile(
	versionToken string,
	semanticVersion string,
	tableNames []string,
	structNames []string,
) error {
	_, filename, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(filename)

	repoRoot := filepath.Dir(filepath.Dir(currentDir))
	dir := filepath.Join(repoRoot, "domain", "export", "state", "model")

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmplPath := filepath.Join(filepath.Dir(filename), "state.tmpl")
	tmplBytes, err := os.ReadFile(tmplPath)
	if err != nil {
		return err
	}

	data := struct {
		VersionToken    string
		SemanticVersion string
		TableNames      []string
		StructNames     []string
	}{
		VersionToken:    versionToken,
		SemanticVersion: semanticVersion,
		TableNames:      tableNames,
		StructNames:     structNames,
	}

	t := template.Must(template.New("state").Parse(string(tmplBytes)))
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return err
	}

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		log.Printf("error formatting generated code for v%s.go: %v", versionToken, err)
		formatted = out.Bytes()
	}

	// Write to stable filenames export.go and export_test.go so the state package
	// always contains the latest export logic.
	filePath := filepath.Join(dir, "export.go")
	fmt.Printf("writing to %s\n", filePath)
	if err := os.WriteFile(filePath, formatted, 0644); err != nil {
		return err
	}

	// Also generate a basic test that runs the ExportV<version> method against
	// the real model DB, written to export_test.go.
	testTmplPath := filepath.Join(filepath.Dir(filename), "state_test.tmpl")
	testTmplBytes, err := os.ReadFile(testTmplPath)
	if err != nil {
		return err
	}

	testData := struct {
		VersionToken string
	}{
		VersionToken: versionToken,
	}

	testT := template.Must(template.New("state_test").Parse(string(testTmplBytes)))
	var testOut bytes.Buffer
	if err := testT.Execute(&testOut, testData); err != nil {
		return err
	}
	testFormatted, err := format.Source(testOut.Bytes())
	if err != nil {
		return err
	}

	testFilePath := filepath.Join(dir, "export_test.go")
	fmt.Printf("writing to %s\n", testFilePath)
	if err := os.WriteFile(testFilePath, testFormatted, 0644); err != nil {
		return err
	}

	return nil
}

func writeServiceModelVersionFile(versionToken, semanticVersion string) error {
	_, filename, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(filename)

	repoRoot := filepath.Dir(filepath.Dir(currentDir))
	dir := filepath.Join(repoRoot, "domain", "export", "service")

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmplPath := filepath.Join(filepath.Dir(filename), "service.tmpl")
	tmplBytes, err := os.ReadFile(tmplPath)
	if err != nil {
		return err
	}

	data := struct {
		VersionToken    string
		SemanticVersion string
	}{
		VersionToken:    versionToken,
		SemanticVersion: semanticVersion,
	}

	t := template.Must(template.New("service").Parse(string(tmplBytes)))
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return err
	}

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return err
	}

	filePath := filepath.Join(dir, "export.go")
	fmt.Printf("writing to %s\n", filePath)
	if err := os.WriteFile(filePath, formatted, 0644); err != nil {
		return err
	}

	testTmplPath := filepath.Join(filepath.Dir(filename), "service_test.tmpl")
	testTmplBytes, err := os.ReadFile(testTmplPath)
	if err != nil {
		return err
	}

	testT := template.Must(template.New("service_test").Parse(string(testTmplBytes)))
	var testOut bytes.Buffer
	if err := testT.Execute(&testOut, data); err != nil {
		return err
	}

	testFormatted, err := format.Source(testOut.Bytes())
	if err != nil {
		return err
	}

	testFilePath := filepath.Join(dir, "export_test.go")
	fmt.Printf("writing to %s\n", testFilePath)
	return os.WriteFile(testFilePath, testFormatted, 0644)
}

// importTableData describes one table the generated importer inserts. Seeded
// marks tables the schema pre-populates, whose inserts use ON CONFLICT DO NOTHING
// so the (identical) seed rows are skipped while genuine content rows are kept.
type importTableData struct {
	StructName string
	TableName  string
	Seeded     bool
}

// writeStateImportFile generates the importer (the write-mirror of the exporter)
// into domain/modelimport/state/model/import.go, plus a smoke test. It is driven
// by import.tmpl / import_test.tmpl and the included (non-bootstrap, non-infra)
// table list.
func writeStateImportFile(
	versionToken string,
	semanticVersion string,
	tables []importTableData,
) error {
	_, filename, _, _ := runtime.Caller(0)
	currentDir := filepath.Dir(filename)

	repoRoot := filepath.Dir(filepath.Dir(currentDir))
	dir := filepath.Join(repoRoot, "domain", "modelimport", "state", "model")

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data := struct {
		VersionToken    string
		SemanticVersion string
		Tables          []importTableData
	}{
		VersionToken:    versionToken,
		SemanticVersion: semanticVersion,
		Tables:          tables,
	}

	if err := renderImportTemplate(filepath.Join(currentDir, "import.tmpl"), filepath.Join(dir, "import.go"), "import", data); err != nil {
		return err
	}
	if err := renderImportTemplate(filepath.Join(currentDir, "import_test.tmpl"), filepath.Join(dir, "import_test.go"), "import_test", data); err != nil {
		return err
	}
	return nil
}

// renderImportTemplate parses the template at tmplPath, executes it with data,
// gofmt-formats the result, and writes it to outPath.
func renderImportTemplate(tmplPath, outPath, name string, data any) error {
	tmplBytes, err := os.ReadFile(tmplPath)
	if err != nil {
		return err
	}

	t := template.Must(template.New(name).Parse(string(tmplBytes)))
	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return err
	}

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return err
	}

	fmt.Printf("writing to %s\n", outPath)
	return os.WriteFile(outPath, formatted, 0644)
}
