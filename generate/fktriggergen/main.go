// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"text/template"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	databaseschema "github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/txn"
)

func main() {
	var (
		dbTypeFlag         = flag.String("db", "controller", "Database type to use (controller|model)")
		destinationPackage = flag.String("package", "schema", "Package name to use")
		destination        = flag.String("destination", "", "Destination directory to write the triggers to")
	)
	flag.Parse()

	path, err := os.MkdirTemp(os.TempDir(), *dbTypeFlag)
	if err != nil {
		panic(err)
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		panic(err)
	}

	dbApp, err := app.New(path)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer cancel()

		select {
		case <-ch:
		case <-ctx.Done():
		}
		dbApp.Close()
	}()

	if err := dbApp.Ready(ctx); err != nil {
		log.Fatalln("waiting for db", err)
	}

	db, err := dbApp.Open(ctx, *dbTypeFlag)
	if err != nil {
		log.Fatalln("cannot open db", err)
	}
	defer db.Close()

	runner, err := initDBRunner(ctx, db, *dbTypeFlag)
	if err != nil {
		log.Fatalln("cannot open db runner", err)
	}

	tableNames, err := listTables(ctx, runner)
	if err != nil {
		log.Fatalln("cannot read table columns", err)
	}

	var allFKs []fk
	for _, tableName := range tableNames {
		fks, err := listFKs(ctx, runner, tableName)
		if err != nil {
			log.Fatalf("cannot read FKs for %s: %v", tableName, err)
		}
		// Flatten composite FK constraints
		grouped := map[string]fk{}
		for _, fkc := range fks {
			existing, ok := grouped[fkc.ID]
			if !ok {
				grouped[fkc.ID] = fkc
				continue
			}
			if fkc.ToTable != existing.ToTable ||
				fkc.Match != existing.Match ||
				fkc.OnDelete != existing.OnDelete ||
				fkc.OnUpdate != existing.OnUpdate {
				log.Fatal("invalid fk value - mismatch")
			}
			if fkc.Seq <= existing.Seq {
				log.Fatal("fk out of sequence")
			}
			merged := fkc
			merged.From = append(existing.From, fkc.From...)
			merged.To = append(existing.To, fkc.To...)
			grouped[fkc.ID] = merged
		}
		allFKs = slices.AppendSeq(allFKs, maps.Values(grouped))
	}

	// Remove complex FK constraints
	allFKs = slices.DeleteFunc(allFKs, func(fkc fk) bool {
		return fkc.OnUpdate != "NO ACTION" ||
			fkc.OnDelete != "NO ACTION" ||
			fkc.Match != "NONE"
	})

	result, err := renderTemplates(allFKs, *destinationPackage)
	if err != nil {
		log.Fatalln("cannot render templates", err)
	}

	file := os.Stdout
	if *destination != "" {
		var err error
		file, err = os.OpenFile(*destination, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalln("cannot open destination file", err)
		}
		defer func() {
			_ = file.Sync()
			_ = file.Close()
		}()
		if err := file.Truncate(0); err != nil {
			log.Fatalln("cannot truncate file", err)
		}
		_, _ = file.Seek(0, io.SeekStart)
	}

	fmt.Fprintln(file, result)

	os.Exit(0)
}

func initDBRunner(ctx context.Context, db *sql.DB, dbType string) (*txnRunner, error) {
	var schema *databaseschema.Schema
	switch dbType {
	case "controller":
		schema = controllerSchema()
	case "model":
		schema = modelSchema()
	default:
		panic("unknown database type")
	}

	runner := &txnRunner{
		db: db,
	}

	if _, err := schema.Ensure(ctx, runner); err != nil {
		return nil, errors.Annotatef(err, "cannot ensure schema")
	}
	return runner, nil
}

type tableListInfo struct {
	schema string
	name   string
	ttype  string
	ncol   int
	wr     bool
	strict bool
}

func listTables(ctx context.Context, runner *txnRunner) ([]string, error) {
	var names []string
	err := runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `PRAGMA table_list;`)
		if err != nil {
			return err
		}
		for rows.Next() {
			t := tableListInfo{}
			err := rows.Scan(
				&t.schema, &t.name, &t.ttype, &t.ncol, &t.wr, &t.strict,
			)
			if err != nil {
				return err
			}
			if t.ttype != "table" {
				continue
			}
			if t.schema != "main" {
				continue
			}
			names = append(names, t.name)
		}
		return rows.Close()
	})
	return names, err
}

type fk struct {
	FromTable string

	ID       string
	Seq      string
	ToTable  string
	From     []string
	To       []string
	OnUpdate string
	OnDelete string
	Match    string
}

func listFKs(ctx context.Context, runner *txnRunner, tableName string) ([]fk, error) {
	var fks []fk
	err := runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, fmt.Sprintf(`PRAGMA foreign_key_list(%q);`, tableName))
		if err != nil {
			return err
		}
		for rows.Next() {
			t := fk{
				FromTable: tableName,
			}
			from := ""
			to := ""
			err := rows.Scan(
				&t.ID, &t.Seq, &t.ToTable, &from, &to,
				&t.OnUpdate, &t.OnDelete, &t.Match,
			)
			if err != nil {
				return err
			}
			t.From = []string{from}
			t.To = []string{to}
			fks = append(fks, t)
		}
		return rows.Close()
	})
	return fks, err
}

func renderTemplates(fks []fk, destPackage string) (string, error) {
	slices.SortFunc(fks, func(a fk, b fk) int {
		return strings.Compare(
			a.FromTable+a.ID,
			b.FromTable+b.ID,
		)
	})

	temp := template.New("trigger")

	builder := new(strings.Builder)
	doc := template.Must(temp.Parse(triggerTemplate))
	if err := doc.Execute(builder, struct {
		FKs     []fk
		Package string
	}{
		FKs:     fks,
		Package: destPackage,
	}); err != nil {
		return "", errors.Annotatef(err, "cannot render template")
	}
	return builder.String(), nil
}

var triggerTemplate = `// Code generated by fktriggergen. DO NOT EDIT.

package {{.Package}}

import (
	"github.com/juju/juju/core/database/schema"
)

// FKDebugTriggers generates triggers from all tables to debug FK violations.
func FKDebugTriggers() func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(` + "`" + `
{{range $fk := .FKs}}
-- fk debug delete trigger for {{.ToTable}} for fk ref from {{.FromTable}}
CREATE TRIGGER trg_fk_debug_{{.FromTable}}_{{.ID}}
BEFORE DELETE ON '{{.ToTable}}' FOR EACH ROW
BEGIN
        SELECT CASE WHEN COUNT(*) > 0 AND (SELECT * FROM pragma_foreign_keys)
                    THEN
                        RAISE(FAIL, 'Foreign Key violation during DELETE FROM {{.ToTable}} due to referencing rows in {{.FromTable}} ON{{range .From}} {{.}}{{end}}')
                    ELSE
                        NULL
                    END panic
        FROM '{{.FromTable}}'
        WHERE {{range $i := len $fk.From}}{{if gt $i 0}} AND {{end}}{{index $fk.From $i}}=OLD.{{index $fk.To $i}}{{end}};
END;
{{end}}
` + "`" + `)
	}
}
`

func controllerSchema() *databaseschema.Schema {
	schema.EnableGenerated = false
	return schema.ControllerDDL()
}

func modelSchema() *databaseschema.Schema {
	schema.EnableGenerated = false
	return schema.ModelDDL()
}

type txnRunner struct {
	db *sql.DB
}

func (r *txnRunner) Txn(ctx context.Context, f func(context.Context, *sqlair.TX) error) error {
	return errors.Trace(Txn(ctx, sqlair.NewDB(r.db), f))
}

func (r *txnRunner) StdTxn(ctx context.Context, f func(context.Context, *sql.Tx) error) error {
	return errors.Trace(StdTxn(ctx, r.db, f))
}

func (r *txnRunner) Dying() <-chan struct{} {
	return make(<-chan struct{})
}

var (
	defaultTransactionRunner = txn.NewRetryingTxnRunner()
)

// Txn executes the input function against the tracked database, using
// the sqlair package. The sqlair package provides a mapping library for
// SQL queries and statements.
// Retry semantics are applied automatically based on transient failures.
// This is the function that almost all downstream database consumers
// should use.
//
// This should not be used directly, instead the TxnRunner should be used to
// handle transactions.
func Txn(ctx context.Context, db *sqlair.DB, fn func(context.Context, *sqlair.TX) error) error {
	return defaultTransactionRunner.Txn(ctx, db, fn)
}

// StdTxn defines a generic txn function for applying transactions on a given
// database. It expects that no individual transaction function should take
// longer than the default timeout.
// There are no retry semantics for running the function.
//
// This should not be used directly, instead the TxnRunner should be used to
// handle transactions.
func StdTxn(ctx context.Context, db *sql.DB, fn func(context.Context, *sql.Tx) error) error {
	return defaultTransactionRunner.StdTxn(ctx, db, fn)
}

type stringslice []string

func (ss *stringslice) Set(s string) error {
	(*ss) = append(*ss, strings.Split(s, ",")...)
	return nil
}

func (ss *stringslice) String() string {
	if len(*ss) <= 0 {
		return "..."
	}
	return strings.Join(*ss, ", ")
}
