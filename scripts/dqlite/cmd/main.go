// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bufio"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/canonical/go-dqlite/client"
	"github.com/canonical/sqlair"
	"github.com/gosuri/uitable"
	"github.com/juju/errors"
	"github.com/peterh/liner"

	databaseschema "github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/txn"
)

var (
	dbTypeFlag = flag.String("db", "controller", "Database type to use (controller|model)")
	dbPathFlag = flag.String("db-path", "", "Path to the database")

	// Prevent the SQL commands from being printed to the console on startup.
	quiteFlag = flag.Bool("q", false, "Quite mode")

	// Having a history of sql commands is useful for debugging and
	// for re-running commands.
	history     = flag.Bool("history", true, "Use history")
	historyFile = flag.String("history-file", historyDir(), "History file location")
)

func historyDir() string {
	root := os.Getenv("XDG_DATA_HOME")
	if root == "" {
		root = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	path := filepath.Join(root, "juju", "repl")
	if err := os.MkdirAll(path, 0755); err != nil {
		panic(err)
	}
	return path
}

func main() {
	flag.Parse()

	var schema *databaseschema.Schema
	switch *dbTypeFlag {
	case "controller":
		schema = controllerSchema()
	case "model":
		schema = modelSchema()
	default:
		panic("unknown database type")
	}
	if !*quiteFlag {
		schema.Hook(func(i int, stmt string) error {
			fmt.Printf("-- Applied patch %d\n%s\n", i, stmt)
			return nil
		})
	}

	var file *os.File
	if *history {
		var err error
		file, err = os.OpenFile(filepath.Join(*historyFile, ".history"), os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			panic(err)
		}
		defer file.Close()
	}

	var path string
	if *dbPathFlag == "" {
		var err error
		path, err = os.MkdirTemp(os.TempDir(), *dbTypeFlag)
		if err != nil {
			panic(err)
		}
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		panic(err)
	}

	fmt.Printf("Opening database at %s\n", path)

	dial := client.DefaultDialFunc

	dbApp, err := app.New(path, app.WithAddress("127.0.0.1:9001"))
	if err != nil {
		panic(err)
	}

	fmt.Printf("0x%x %s\n", dbApp.ID(), dbApp.Address())

	if err := dbApp.Ready(context.Background()); err != nil {
		panic(err)
	}

	db, err := dbApp.Open(context.Background(), "tmp")
	if err != nil {
		panic(err)
	}

	if _, err := schema.Ensure(context.Background(), &txnRunner{
		db: db,
	}); err != nil {
		panic(err)
	}

	address := dbApp.Address()
	if address == "" {
		address = "127.0.0.1:9001"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer cancel()

		<-ch
		db.Close()
		dbApp.Close()
	}()

	go func() {
		defer cancel()

		line := liner.NewLiner()
		defer line.Close()

		// Only load history if the flag is set.
		if file != nil {
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line.AppendHistory(scanner.Text())
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			input, err := line.Prompt("dqlite> ")
			if err != nil {
				if err == io.EOF {
					break
				}
				return
			}
			if input == ".exit" {
				return
			}
			if strings.Index(input, ".dump") == 0 {
				path := strings.TrimSpace(input[5:])
				if err := dumpDB(ctx, dial, db, address, path); err != nil {
					fmt.Fprintln(os.Stderr, "Error: ", err)
				}
				return
			}

			result, err := processQuery(ctx, db, input)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error: ", err)
			} else {
				line.AppendHistory(input)
				if result != "" {
					fmt.Println()
					fmt.Println(result)
				}

				// This can fail, so we don't care if it errors out.
				if file != nil {
					fmt.Fprintln(file, input)
					_ = file.Sync()
				}
			}
		}
	}()

	<-ctx.Done()
}

func dumpDB(ctx context.Context, dial client.DialFunc, db *sql.DB, address, dir string) error {
	cli, err := client.New(ctx, address, client.WithDialFunc(dial))
	if err != nil {
		return fmt.Errorf("client.New failed %w", err)
	}

	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("os.Getwd() failed %w", err)
		}
		dir = filepath.Join(dir, "dump")

	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("os.MkdirAll() failed %w", err)
	}

	files, err := cli.Dump(ctx, "tmp")
	if err != nil {
		return fmt.Errorf("dump failed")
	}

	for _, file := range files {
		path := filepath.Join(dir, file.Name)

		err := os.WriteFile(path, file.Data, 0600)
		if err != nil {
			return fmt.Errorf("WriteFile failed on path %s", path)
		}
	}

	return nil
}

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

func processQuery(ctx context.Context, db *sql.DB, line string) (string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin transaction: %w", err)
	}

	rows, err := tx.Query(line)
	if err != nil {
		err = fmt.Errorf("query: %w", err)
		if rbErr := tx.Rollback(); rbErr != nil {
			return "", fmt.Errorf("unable to rollback: %v", err)
		}
		return "", err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		err = fmt.Errorf("columns: %w", err)
		if rbErr := tx.Rollback(); rbErr != nil {
			return "", fmt.Errorf("unable to rollback: %v", err)
		}
		return "", err
	}
	n := len(columns)

	table := uitable.New()
	table.Wrap = true
	table.MaxColWidth = 120

	p := func(values ...any) {
		table.AddRow(values...)
	}
	var cols []any
	for i, column := range columns {
		if strings.Contains(column, "date") || strings.Contains(column, "time") {
			table.RightAlign(i)
		}

		cols = append(cols, strings.ToUpper(column))
	}
	p(cols...)
	p()

	for rows.Next() {
		row := make([]interface{}, n)
		rowPointers := make([]interface{}, n)
		for i := range row {
			rowPointers[i] = &row[i]
		}

		if err := rows.Scan(rowPointers...); err != nil {
			err = fmt.Errorf("scan: %w", err)
			if rbErr := tx.Rollback(); rbErr != nil {
				return "", fmt.Errorf("unable to rollback: %v", err)
			}
			return "", err
		}

		p(row...)
	}

	if err := rows.Err(); err != nil {
		err = fmt.Errorf("rows: %w", err)
		if rbErr := tx.Rollback(); rbErr != nil {
			return "", fmt.Errorf("unable to rollback: %v", err)
		}
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	return table.String(), nil
}
