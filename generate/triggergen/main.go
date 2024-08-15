// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sort"
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
		tables = stringslice{}

		dbTypeFlag         = flag.String("db", "controller", "Database type to use (controller|model)")
		destinationPackage = flag.String("package", "schema", "Package name to use")
		destination        = flag.String("destination", "", "Destination directory to write the triggers to")
	)
	flag.Var(&tables, "tables", "Tables to generate triggers for")
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

	tableColumns, err := readTableColumns(ctx, runner, tables)
	if err != nil {
		log.Fatalln("cannot read table columns", err)
	}

	result, err := renderTemplates(tableColumns, *destinationPackage)
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

func readTableColumns(ctx context.Context, runner *txnRunner, tables []string) (map[string][]columnInfo, error) {
	tableColumns := make(map[string][]columnInfo)
	if err := runner.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, table := range tables {
			columns, err := func(table string) ([]string, error) {
				rows, err := tx.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s", table))
				if err != nil {
					return nil, errors.Annotatef(err, "cannot get columns for table %q", table)
				}
				defer rows.Close()

				columns, err := rows.Columns()
				if err != nil {
					return nil, errors.Annotatef(err, "cannot get column names for table %q", table)
				}

				return columns, nil
			}(table)
			if err != nil {
				return err
			}

			info, err := readTableInfo(ctx, tx, table)
			if err != nil {
				return err
			}

			columnInfos := make([]columnInfo, 0)
			for _, column := range columns {
				info, ok := info[column]
				if !ok {
					return errors.Errorf("column %q not found in table %q", column, table)
				}

				// We don't want to generate triggers for primary keys.
				if info.PK > 0 {
					continue
				}

				columnInfos = append(columnInfos, columnInfo{
					Name:       column,
					Type:       info.Type,
					AllowsNull: info.NotNull == 0,
				})
			}

			tableColumns[table] = columnInfos
		}
		return nil
	}); err != nil {
		return nil, errors.Annotatef(err, "cannot read table columns")
	}
	return tableColumns, nil
}

func readTableInfo(ctx context.Context, tx *sql.Tx, table string) (map[string]tableInfo, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_xinfo(%s);", table))
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get column info for table %q", table)
	}
	defer rows.Close()

	info := make(map[string]tableInfo)
	for rows.Next() {
		var tableInfo tableInfo
		if err := rows.Scan(
			&tableInfo.CID,
			&tableInfo.Name,
			&tableInfo.Type,
			&tableInfo.NotNull,
			&tableInfo.Default,
			&tableInfo.PK,
			&tableInfo.Hidden,
		); err != nil {
			return nil, errors.Annotatef(err, "cannot scan column info for table %q", table)
		}

		info[tableInfo.Name] = tableInfo
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Annotatef(err, "cannot iterate column info for table %q", table)
	}

	return info, nil
}

func renderTemplates(tableColumns map[string][]columnInfo, destPackage string) (string, error) {
	view := make([]tableData, 0)
	for table, columnInfos := range tableColumns {
		view = append(view, tableData{
			Name:        table,
			ColumnInfos: columnInfos,
		})
	}
	sort.Slice(view, func(i, j int) bool {
		return view[i].Name < view[j].Name
	})

	temp := template.New("trigger")
	temp.Funcs(template.FuncMap{
		"title": func(s string) string {
			b := new(strings.Builder)
			for i := 0; i < len(s); i++ {
				if s[i] == '_' {
					if i+1 < len(s) {
						b.WriteString(strings.ToUpper(s[i+1 : i+2]))
						i++
					}
					continue
				}
				b.WriteString(s[i : i+1])
			}
			s = b.String()

			return strings.ToUpper(s[:1]) + s[1:]
		},
		"notLast": func(index, total int) bool {
			return index < total-1
		},
		"generateUpdateCompare": func(info columnInfo) string {
			if info.AllowsNull {
				return fmt.Sprintf("(NEW.%[1]s != OLD.%[1]s OR (NEW.%[1]s IS NOT NULL AND OLD.%[1]s IS NULL) OR (NEW.%[1]s IS NULL AND OLD.%[1]s IS NOT NULL))", info.Name)
			}
			return fmt.Sprintf("NEW.%[1]s != OLD.%[1]s", info.Name)
		},
	})

	builder := new(strings.Builder)
	doc := template.Must(temp.Parse(triggerTemplate))
	if err := doc.Execute(builder, struct {
		Views   []tableData
		Package string
	}{
		Views:   view,
		Package: destPackage,
	}); err != nil {
		return "", errors.Annotatef(err, "cannot render template")
	}
	return builder.String(), nil
}

type tableInfo struct {
	CID     int
	Name    string
	Type    string
	NotNull int
	Default *string
	PK      int
	Hidden  int
}

type columnInfo struct {
	Name       string
	Type       string
	AllowsNull bool
}

type tableData struct {
	Name        string
	ColumnInfos []columnInfo
}

var triggerTemplate = `// Code generated by triggergen. DO NOT EDIT.

package {{.Package}}

import (
	"fmt"
	"strings"

	"github.com/juju/juju/core/database/schema"
)

{{range .Views}}
// ChangeLogTriggersFor{{title .Name}} generates the triggers for the
// {{.Name}} table.
func ChangeLogTriggersFor{{title .Name}}(namespaceID int, changeColumnName string) func() schema.Patch {
	return ChangeLogTriggersFor{{title .Name}}WithDiscriminator(namespaceID, changeColumnName, "")
}

// ChangeLogTriggersFor{{title .Name}}WithDiscriminator generates the triggers for the
// {{.Name}} table, with the value of the optional discriminator column included in the
// change event. The discriminator column name is ignored if empty.
func ChangeLogTriggersFor{{title .Name}}WithDiscriminator(namespaceID int, changeColumnName, discriminatorColumnName string) func() schema.Patch {
	changeLogColumns := []string{"changed"}
	newColumnValues := "NEW." + changeColumnName
	oldColumnValues := "OLD." + changeColumnName
	if discriminatorColumnName != "" {
		changeLogColumns = append(changeLogColumns, "discriminator")
		newColumnValues += ", NEW." + discriminatorColumnName
		oldColumnValues += ", OLD." + discriminatorColumnName
	}
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(` + "`" + `
-- insert trigger for {{title .Name}}
CREATE TRIGGER trg_log_{{.Name}}_insert
AFTER INSERT ON {{.Name}} FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, %[4]s, created_at)
    VALUES (1, %[1]d, %[2]s, DATETIME('now'));
END;
{{$total := len .ColumnInfos}}
-- update trigger for {{title .Name}}
CREATE TRIGGER trg_log_{{.Name}}_update
AFTER UPDATE ON {{.Name}} FOR EACH ROW
WHEN {{range $index, $column := .ColumnInfos}}
	{{ (generateUpdateCompare $column) }} {{if (notLast $index $total)}}OR{{end}}{{end}}
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, %[4]s, created_at)
    VALUES (2, %[1]d, %[3]s, DATETIME('now'));
END;

-- delete trigger for {{title .Name}}
CREATE TRIGGER trg_log_{{.Name}}_delete
AFTER DELETE ON {{.Name}} FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, %[4]s, created_at)
    VALUES (4, %[1]d, %[3]s, DATETIME('now'));
END;` + "`" + `, namespaceID, newColumnValues, oldColumnValues, strings.Join(changeLogColumns, ", ")))
	}
}
{{end}}`

func controllerSchema() *databaseschema.Schema {
	return schema.ControllerDDL()
}

func modelSchema() *databaseschema.Schema {
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
