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
// itself when it bootstraps the model DB during a v8 import. They must never be
// inserted from the source payload. model_agent is intentionally not listed:
// it is merged into the bootstrap row by a generated special case.
var bootstrapTables = map[string]bool{
	"model":           true,
	"model_life":      true,
	"agent_version":   true,
	"model_migrating": true,
}

// nonContentTables are tables the generated importer must not populate from
// the YAML payload, because their rows are not portable model content:
//
//   - changestream tables (change_log*): the target's changestream starts
//     fresh.
//   - binary-residency tables: these describe binaries/blobs that transfer
//     over the separate /migrate/{charms,tools,resources} HTTP endpoints, not
//     in the YAML payload. The binary-transfer phase that runs after import
//     re-establishes them once each blob has actually landed on this
//     controller. Importing the source's rows is always wrong (they describe
//     the source's blobs/object-store, not this controller's), and for most
//     of them it is also a hard failure: the transfer phase's own insert
//     collides with the row already carried over from the source (verified
//     live for charm_hash and agent_binary_store; the same PK/lookup-based
//     collision applies to the object_store_* and resource_*_store tables
//     below).
//
// The seeded changestream tables (change_log_edit_type, change_log_namespace)
// are also auto-excluded by getSeededTables; they are listed here for clarity.
//
// NOTE: this list (and bootstrapTables) is the import-exclusion contract.
var nonContentTables = map[string]bool{
	"change_log":           true,
	"change_log_witness":   true,
	"change_log_edit_type": true,
	"change_log_namespace": true,

	// charm binary residency: charm_hash is insert-only (its "unmodifiable"
	// trigger blocks UPDATE), so the binary-transfer phase's re-insert of the
	// verified hash collides with the row carried over from the source.
	"charm_hash": true,

	// agent-binary (tools) residency: agent_binary_store's PK is
	// (version, architecture_id) with no ON CONFLICT in the generated
	// insert; the /migrate/tools transfer phase re-registers the binary on
	// upload and collides with the imported row. Reproduced live as
	// "agent binary already exists for version ... and arch ...".
	"agent_binary_store": true,

	// object-store residency: all three describe where a blob physically
	// sits, keyed by the source's object-store UUID and (for
	// object_store_metadata_path) a path that is deterministic for agent
	// binaries, so re-uploading the same tool collides on the path PK.
	// object_store_placement additionally records the *source* controller
	// node as the blob's location, which is simply wrong on the target.
	"object_store_metadata":      true,
	"object_store_metadata_path": true,
	"object_store_placement":     true,

	// resource binary residency: same pattern as the charm/tools cases. Each
	// store table's writer pre-checks for an existing row keyed by the
	// resource's UUID (preserved across import) and fails with
	// StoredResourceAlreadyExists / ContainerImageMetadataAlreadyStored when
	// the transfer phase re-uploads the resource.
	"resource_file_store":                     true,
	"resource_image_store":                    true,
	"resource_container_image_metadata_store": true,

	// operation_task_output.store_path is a NOT NULL FK into
	// object_store_metadata_path, which is itself excluded above (object-store
	// residency). Unlike charm's blob-residency columns, this column can't be
	// sanitized to NULL instead of being imported: it's mandatory, so any task
	// with recorded output would otherwise fail the deferred foreign-key check
	// at commit. The output blob itself never rode in the YAML payload (it
	// lives in the object store), so there is nothing valid to carry over.
	"operation_task_output": true,
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
		fmt.Fprintf(os.Stderr, "failed to generate model import: %v\n", err)
		os.Exit(1)
	}
}

func generate(ctx context.Context, runner *txnRunner) error {
	if len(export.ExportVersions) == 0 {
		return fmt.Errorf("no export versions defined")
	}
	semanticVersion := slices.MaxFunc(export.ExportVersions, semversion.Number.Compare).String()
	versionToken := strings.ReplaceAll(semanticVersion, ".", "_")

	tableNames, err := getTableNames(ctx, runner)
	if err != nil {
		return err
	}
	seeded, err := getSeededTables(ctx, runner, tableNames)
	if err != nil {
		return err
	}

	var importTables []importTableData
	var hasModelAgent bool
	for _, tableName := range tableNames {
		if tableName == "sqlite_sequence" {
			continue
		}
		switch {
		case tableName == "model_agent":
			hasModelAgent = true
			continue
		case bootstrapTables[tableName] || nonContentTables[tableName]:
			continue
		}
		importTables = append(importTables, importTableData{
			StructName: toCamelCase(tableName),
			TableName:  tableName,
			Seeded:     seeded[tableName],
		})
	}
	if !hasModelAgent {
		return fmt.Errorf("model_agent table not found")
	}

	return writeStateImportFile(versionToken, semanticVersion, importTables)
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

// getSeededTables returns the set of tables that already contain rows after the
// schema has been applied to the empty in-memory DB. These are the lookup tables
// the schema seeds (life, secret_role, ip_address_type, ...). The target seeds
// the same rows, so the importer must not re-insert them.
func getSeededTables(ctx context.Context, runner *txnRunner, tableNames []string) (map[string]bool, error) {
	var seeded map[string]bool
	err := runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		seeded = make(map[string]bool)
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

// importTableData describes one table the generated importer inserts. Seeded
// marks tables the schema pre-populates, whose inserts use ON CONFLICT DO
// NOTHING so the identical seed rows are skipped while genuine content rows are
// kept.
type importTableData struct {
	StructName string
	TableName  string
	Seeded     bool
}

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

	if err := renderTemplate(filepath.Join(currentDir, "import.tmpl"), filepath.Join(dir, "import.go"), "import", data); err != nil {
		return err
	}
	if err := renderTemplate(filepath.Join(currentDir, "import_test.tmpl"), filepath.Join(dir, "import_test.go"), "import_test", data); err != nil {
		return err
	}
	return nil
}

func renderTemplate(tmplPath, outPath, name string, data any) error {
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
