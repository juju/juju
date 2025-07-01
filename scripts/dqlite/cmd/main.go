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

	"github.com/canonical/go-dqlite/v2/client"
	"github.com/canonical/sqlair"
	"github.com/gosuri/uitable"
	"github.com/juju/errors"
	"github.com/peterh/liner"

	databaseschema "github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/domain/schema"
	"github.com/juju/juju/internal/database/app"
	"github.com/juju/juju/internal/database/txn"
)

const (
	unixSocketAddress = "@dqlite-repl"
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
		schema.Hook(func(i int, stmt string) (string, error) {
			fmt.Printf("-- Applying patch %d\n%s\n", i, stmt)
			return stmt, nil
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
	} else {
		path = *dbPathFlag
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		panic(err)
	}

	dbApp, err := app.New(path, app.WithAddress(unixSocketAddress))
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer cancel()

		<-ch
		dbApp.Close()
	}()

	if err := dbApp.Ready(ctx); err != nil {
		panic(err)
	}

	db, err := dbApp.Open(ctx, *dbTypeFlag)
	if err != nil {
		panic(err)
	}

	if _, err := schema.Ensure(ctx, &txnRunner{
		db: db,
	}); err != nil {
		panic(err)
	}

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
				if err := dumpDB(ctx, db, path, *dbTypeFlag); err != nil {
					fmt.Fprintln(os.Stderr, "Error: ", err)
				}
				return
			}

			// Process the input query.
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

func dumpDB(ctx context.Context, db *sql.DB, path, name string) error {
	cli, err := client.New(ctx, unixSocketAddress)
	if err != nil {
		return fmt.Errorf("client.New failed %w", err)
	}

	files, err := cli.Dump(ctx, name)
	if err != nil {
		return fmt.Errorf("dump failed")
	}

	for _, file := range files {
		filePath := filepath.Join(path, file.Name)
		fmt.Println("Dumping file", path)

		err := os.WriteFile(filePath, file.Data, 0600)
		if err != nil {
			return fmt.Errorf("WriteFile failed on path %s", filePath)
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
