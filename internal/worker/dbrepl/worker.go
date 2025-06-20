// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbrepl

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/juju/ansiterm"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/database/client"
	"github.com/juju/juju/internal/database/dqlite"
	"github.com/juju/juju/internal/worker"
)

// NodeManager creates Dqlite `App` initialisation arguments and options.
type NodeManager interface {
	// ClusterServers returns the node information for
	// Dqlite nodes configured to be in the cluster.
	ClusterServers(context.Context) ([]dqlite.NodeInfo, error)

	// WithTLSDialer returns a dbApp that can be used to connect to the Dqlite
	// cluster.
	WithTLSDialer(ctx context.Context) (client.DialFunc, error)
}

// DBGetter describes the ability to supply a sql.DB
// reference for a particular database.
type DBGetter interface {
	// GetDB returns a sql.DB reference for the dqlite-backed database that
	// contains the data for the specified namespace.
	// A NotFound error is returned if the worker is unaware of the requested DB.
	GetDB(namespace string) (database.TxnRunner, error)
}

// WorkerConfig encapsulates the configuration options for the
// dbaccessor worker.
type WorkerConfig struct {
	DBGetter database.DBGetter
	Logger   logger.Logger
	Stdout   io.Writer
	Stderr   io.Writer
	Stdin    io.Reader
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.DBGetter == nil {
		return errors.NotValidf("missing DBGetter")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.Stdout == nil {
		return errors.NotValidf("missing Stdout")
	}
	if c.Stderr == nil {
		return errors.NotValidf("missing Stderr")
	}
	if c.Stdin == nil {
		return errors.NotValidf("missing Stdin")
	}
	return nil
}

type dbReplWorker struct {
	cfg  WorkerConfig
	tomb tomb.Tomb

	dbGetter         database.DBGetter
	controllerDB     database.TxnRunner
	currentDB        database.TxnRunner
	currentNamespace string
}

// NewWorker creates a new dbaccessor worker.
func NewWorker(cfg WorkerConfig) (*dbReplWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	controllerDB, err := cfg.DBGetter.GetDB(database.ControllerNS)
	if err != nil {
		return nil, errors.Annotate(err, "getting controller db")
	}

	w := &dbReplWorker{
		cfg:              cfg,
		dbGetter:         cfg.DBGetter,
		controllerDB:     controllerDB,
		currentDB:        controllerDB,
		currentNamespace: "*",
	}

	w.tomb.Go(w.loop)

	return w, nil
}

func (w *dbReplWorker) loop() (err error) {
	ctx, cancel := w.scopedContext()
	defer cancel()

	history, err := os.CreateTemp("", "juju-repl")
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		_ = history.Close()
		if err := os.Remove(history.Name()); err != nil {
			w.cfg.Logger.Errorf(ctx, "failed to remove history file: %v", err)
		}
	}()

	line, err := readline.NewEx(&readline.Config{
		Stdin:               readline.NewCancelableStdin(w.cfg.Stdin),
		Stdout:              w.cfg.Stdout,
		Stderr:              w.cfg.Stderr,
		HistoryFile:         history.Name(),
		InterruptPrompt:     "^C",
		HistorySearchFold:   true,
		FuncFilterInputRune: filterInput,
	})
	if err != nil {
		return errors.Trace(err)
	}
	defer line.Close()

	// TODO (stickupkid): If we're not in a tty, then just write "connecting" to
	// stdout.
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Millisecond * 500)
		defer ticker.Stop()

		var amount int
		for {
			select {
			case <-done:
				return
			case <-w.tomb.Dying():
				return
			case <-ticker.C:
				if amount > 0 {
					fmt.Fprint(w.cfg.Stdout, "\033[1A\033[K")
				}
				fmt.Fprintln(w.cfg.Stdout, "connecting", strings.Repeat(".", amount%4))
				amount++
			}
		}
	}()

	currentDB, err := w.dbGetter.GetDB(database.ControllerNS)
	if err != nil {
		return errors.Annotate(err, "failed to get db")
	}
	w.controllerDB = currentDB
	w.currentNamespace = "controller"

	close(done)

	// Allow the line to be closed when the worker is dying.
	go func() {
		defer line.Close()

		select {
		case <-w.tomb.Dying():
			return
		case <-ctx.Done():
			return
		}
	}()

	// Run the main REPL loop.
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		default:
		}

		line.SetPrompt("repl (" + w.currentNamespace + ")> ")
		if err != nil {
			return errors.Annotate(err, "failed to read input")
		}

		input, err := line.Readline()
		if err == readline.ErrInterrupt {
			if len(input) == 0 {
				return nil
			} else {
				continue
			}
		} else if err == io.EOF {
			return nil
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		args := strings.Split(input, " ")
		if len(args) == 0 {
			continue
		}

		switch args[0] {
		case ".exit", ".quit":
			return worker.ErrTerminateAgent
		case ".help", ".h":
			fmt.Fprint(w.cfg.Stdout, helpText)
		case ".switch":
			w.execSwitch(ctx, args[1:])
		case ".models":
			w.execModels(ctx)
		case ".tables":
			w.execTables(ctx)
		case ".triggers":
			w.execTriggers(ctx)
		case ".views":
			w.execViews(ctx)
		case ".ddl":
			w.execShowDDL(ctx, args[1:])
		case ".query-models":
			w.execQueryForModels(ctx, args[1:])

		default:
			if err := w.executeQuery(ctx, w.currentDB, input); err != nil {
				w.cfg.Logger.Errorf(ctx, "failed to execute query: %v", err)
			}
		}
	}
}

func (w *dbReplWorker) execSwitch(ctx context.Context, args []string) {
	if len(args) != 1 {
		fmt.Fprintln(w.cfg.Stderr, "usage: .switch model-<name> or .switch controller for global controller database")
		return
	}

	argName := args[0]
	if argName == "controller" {
		w.currentDB = w.controllerDB
		w.currentNamespace = argName
		return
	}

	name, ok := strings.CutPrefix(argName, "model-")
	if !ok {
		fmt.Fprintln(w.cfg.Stderr, `invalid model namespace name: expected "model-<name>" or "controller"`)
		return
	}

	var uuid string
	if err := w.controllerDB.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, "SELECT uuid FROM model WHERE name=?", name)
		if err := row.Scan(&uuid); err != nil {
			return err
		}
		return nil
	}); errors.Is(err, sql.ErrNoRows) {
		fmt.Fprintf(w.cfg.Stderr, "model %q not found\n", name)
		return
	} else if err != nil {
		fmt.Fprintf(w.cfg.Stderr, "failed to select %q database: %v\n", name, err)
		return
	}

	var err error
	w.currentDB, err = w.dbGetter.GetDB(uuid)
	if err != nil {
		fmt.Fprintf(w.cfg.Stderr, "failed to switch to namespace %q: %v\n", name, err)
		return
	}
	w.currentNamespace = argName
}

func (w *dbReplWorker) execModels(ctx context.Context) {
	if err := w.executeQuery(ctx, w.controllerDB, "SELECT uuid, name FROM model"); err != nil {
		w.cfg.Logger.Errorf(ctx, "failed to execute query: %v", err)
	}
}

func (w *dbReplWorker) execQueryForModels(ctx context.Context, args []string) {
	var models []string
	if err := w.controllerDB.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT uuid FROM model")
		if err != nil {
			return err
		}

		defer rows.Close()

		for rows.Next() {
			var uuid string
			if err := rows.Scan(&uuid); err != nil {
				return err
			}
			models = append(models, uuid)
		}

		return nil
	}); err != nil {
		w.cfg.Logger.Errorf(ctx, "failed to execute query: %v", err)
		return
	}

	for _, model := range models {
		db, err := w.dbGetter.GetDB(model)
		if err != nil {
			w.cfg.Logger.Errorf(ctx, "failed to get db for model %q: %v", model, err)
			continue
		}

		str := "Executing query on model: " + model
		fmt.Fprintln(w.cfg.Stdout, str)
		fmt.Fprintln(w.cfg.Stdout, strings.Repeat("-", len(str)))
		query := strings.Join(args, " ")
		if err := w.executeQuery(ctx, db, query); err != nil {
			w.cfg.Logger.Errorf(ctx, "failed to execute query on model %q: %v", model, err)
		}
	}
}

func (w *dbReplWorker) execTables(ctx context.Context) {
	if err := w.executeQuery(ctx, w.currentDB, "SELECT name AS table_name FROM sqlite_master WHERE type='table'"); err != nil {
		w.cfg.Logger.Errorf(ctx, "failed to execute query: %v", err)
	}
}

func (w *dbReplWorker) execShowDDL(ctx context.Context, args []string) {
	if len(args) != 1 {
		fmt.Fprintln(w.cfg.Stderr, "usage: .ddl <name>")
		return
	}

	name := args[0]
	var ddl string
	if err := w.currentDB.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE name=?", name)
		if err := row.Scan(&ddl); err != nil {
			return err
		}
		return nil
	}); err != nil {
		w.cfg.Logger.Errorf(ctx, "failed to execute query: %v\n", err)
	}

	fmt.Fprintln(w.cfg.Stdout, ddl)
}

func (w *dbReplWorker) execTriggers(ctx context.Context) {
	if err := w.executeQuery(ctx, w.currentDB, "SELECT name AS trigger_name FROM sqlite_master WHERE type='trigger'"); err != nil {
		w.cfg.Logger.Errorf(ctx, "failed to execute query: %v", err)
	}
}

func (w *dbReplWorker) execViews(ctx context.Context) {
	if err := w.executeQuery(ctx, w.currentDB, "SELECT name AS view_name FROM sqlite_master WHERE type='view'"); err != nil {
		w.cfg.Logger.Errorf(ctx, "failed to execute query: %v", err)
	}
}

// Kill is part of the worker.Worker interface.
func (w *dbReplWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *dbReplWorker) Wait() error {
	return w.tomb.Wait()
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *dbReplWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.tomb.Context(ctx), cancel
}

func (w *dbReplWorker) executeQuery(ctx context.Context, db database.TxnRunner, query string) error {
	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return err
		}

		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			return err
		}
		n := len(columns)

		headerStyle := color.New(color.Bold)
		var sb strings.Builder

		// Use the ansiterm tabwriter because the stdlib tabwriter contains a bug
		// which breaks if there are color codes. Our own tabwriter implementation
		// doesn't have this issue.
		writer := ansiterm.NewTabWriter(&sb, 0, 8, 1, '\t', 0)
		for _, col := range columns {
			headerStyle.Fprintf(writer, "%s\t", col)
		}
		fmt.Fprintln(writer)

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
				fmt.Fprintf(writer, "%v\t", column)
			}
			fmt.Fprintln(writer)
		}

		if err := rows.Err(); err != nil {
			return err
		}

		if err := writer.Flush(); err != nil {
			return err
		}

		fmt.Fprintln(w.cfg.Stdout, sb.String())

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

const helpText = `
The following commands are available:

  .exit, .quit             Exit the REPL.
  .help, .h                Show this help message.
  .models                  Show all models.
  .switch model-<model>    Switch to a different model.
  .switch controller       Switch to the controller global database.
  .tables                  Show all standard tables in the current database.
  .triggers                Show all trigger tables in the current database.
  .views                   Show all views in the current database.
  .ddl <name>              Show the DDL for the specified table, trigger, or view.
  .query-models <query>    Execute a query on all models and print the results.

`
