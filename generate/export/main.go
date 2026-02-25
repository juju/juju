// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:generate go run main.go

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
	"sort"
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
	// Use the last listed export version string (e.g., "4.0.1").
	if len(export.ExportVersions) == 0 {
		return fmt.Errorf("no export versions defined")
	}
	semanticVersion := export.ExportVersions[len(export.ExportVersions)-1]

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

	if err := writeTypesFile(versionToken, structs, structNames, imports); err != nil {
		return err
	}

	if err := writeStateModelVersionFile(versionToken, semanticVersion, usedTableNames, structNames); err != nil {
		return err
	}

	return writeServiceModelVersionFile(versionToken, semanticVersion)
}

func getTableNames(ctx context.Context, runner *txnRunner) ([]string, error) {
	var tableNames []string
	err := runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
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

		if _, err := sb.WriteString(fmt.Sprintf("\t%s %s `db:%q`\n", fieldName, goType, col.Name)); err != nil {
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

func writeTypesFile(version string, structs []string, structNames []string, imports map[string]struct{}) error {
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
		Structs     []string
		StructNames []string
	}{
		Version:     version,
		Imports:     sortedImports,
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
	if err := os.WriteFile(testFilePath, testFormatted, 0644); err != nil {
		return err
	}

	return nil
}
