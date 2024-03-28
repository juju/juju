// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

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
)

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
	schema.Hook(func(i int) error {
		fmt.Printf("-- Applied patch %d\n", i)
		return nil
	})

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

	dbApp, err := app.New(path)
	if err != nil {
		panic(err)
	}

	if err := dbApp.Ready(context.Background()); err != nil {
		panic(err)
	}

	db, err := dbApp.Open(context.Background(), *dbTypeFlag)
	if err != nil {
		panic(err)
	}

	if _, err := schema.Ensure(context.Background(), &txnRunner{
		db: db,
	}); err != nil {
		panic(err)
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
			if input == "exit" {
				return
			}

			result, err := processQuery(ctx, db, input)
			if err != nil {
				fmt.Println("Error: ", err)
			} else {
				line.AppendHistory(input)
				if result != "" {
					fmt.Println()
					fmt.Println(result)
				}
			}
		}
	}()

	<-ctx.Done()
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
