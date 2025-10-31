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
	"path/filepath"
	"strings"
	"syscall"

	"github.com/canonical/go-dqlite/v2/client"
	"github.com/canonical/sqlair"
	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/juju/ansiterm"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
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
	quietFlag = flag.Bool("q", false, "Quiet mode")

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
	if !*quietFlag {
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

	runner := &txnRunner{
		db: db,
	}

	if _, err := schema.Ensure(ctx, runner); err != nil {
		panic(err)
	}

	go func() {
		defer cancel()

		history, err := os.CreateTemp("", "juju-repl")
		if err != nil {
			panic(err)
		}
		defer func() {
			_ = history.Close()
			if err := os.Remove(history.Name()); err != nil {
				fmt.Fprintf(os.Stderr, "failed to remove history file: %v\n", err)
			}
		}()

		line, err := readline.NewEx(&readline.Config{
			Stdin:               readline.NewCancelableStdin(os.Stdin),
			Stdout:              os.Stdout,
			Stderr:              os.Stderr,
			HistoryFile:         history.Name(),
			InterruptPrompt:     "^C",
			HistorySearchFold:   true,
			FuncFilterInputRune: filterInput,
		})
		if err != nil {
			panic(err)
		}
		defer func() { _ = line.Close() }()

		lineBuildUp := new(strings.Builder)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if lineBuildUp.Len() == 0 {
				line.SetPrompt("dqlite> ")
			} else {
				line.SetPrompt("... ")
			}

			input, err := line.Readline()
			if err == readline.ErrInterrupt {
				if len(input) == 0 {
					return
				} else {
					continue
				}
			} else if err == io.EOF {
				return
			}

			input = strings.TrimSpace(input)
			if input == "" {
				continue
			} else if strings.HasSuffix(input, "\\") {
				_, _ = lineBuildUp.WriteString(input[:len(input)-1])
				_, _ = lineBuildUp.WriteString(" ")
				continue
			}
			_, _ = lineBuildUp.WriteString(input)

			compiled := lineBuildUp.String()

			args := strings.Split(compiled, " ")
			if len(args) == 0 {
				continue
			}

			switch args[0] {
			case ".exit", ".quit":
				return
			case ".dump":
				if err := dumpDB(ctx, db, path, *dbTypeFlag); err != nil {
					fmt.Fprintln(os.Stderr, "Error: ", err)
				}
			default:

				// Process the input query.
				if err := processQuery(ctx, runner, lineBuildUp.String()); err != nil {
					fmt.Fprintln(os.Stderr, "Error: ", err)
				}
			}

			lineBuildUp.Reset()

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
		fmt.Println("Dumping file", filePath)

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

func processQuery(ctx context.Context, db database.TxnRunner, query string, args ...any) error {
	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}

		defer func() { _ = rows.Close() }()

		columns, err := rows.Columns()
		if err != nil {
			return err
		}
		n := len(columns)

		headerStyle := color.New(color.Bold)
		var sb strings.Builder

		// Use the ansiterm tabwriter because the stdlib tabwriter contains a
		// bug which breaks if there are color codes. Our own tabwriter
		// implementation doesn't have this issue.
		writer := ansiterm.NewTabWriter(&sb, 0, 8, 1, '\t', 0)
		for _, col := range columns {
			_, _ = headerStyle.Fprintf(writer, "%s\t", col)
		}
		_, _ = fmt.Fprintln(writer)

		for rows.Next() {
			row := make([]interface{}, n)
			rowPointers := make([]interface{}, n)
			for i := range row {
				rowPointers[i] = &row[i]
			}

			if err := rows.Scan(rowPointers...); err != nil {
				return err
			}

			for _, column := range row {
				_, _ = fmt.Fprintf(writer, "%v\t", column)
			}
			_, _ = fmt.Fprintln(writer)
		}

		if err := rows.Err(); err != nil {
			return err
		}

		if err := writer.Flush(); err != nil {
			return err
		}

		_, _ = fmt.Fprintln(os.Stdout, sb.String())

		return err
	})
}

// filterInput is used to exclude characters
// from being accepted from stdin.
func filterInput(r rune) (rune, bool) {
	switch r {
	// block CtrlZ feature
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}
