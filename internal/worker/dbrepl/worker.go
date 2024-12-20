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
	"text/tabwriter"
	"time"

	"github.com/chzyer/readline"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/database"
	coredatabase "github.com/juju/juju/core/database"
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
	DBGetter coredatabase.DBGetter
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

	dbGetter coredatabase.DBGetter
}

// NewWorker creates a new dbaccessor worker.
func NewWorker(cfg WorkerConfig) (*dbReplWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &dbReplWorker{
		cfg:      cfg,
		dbGetter: cfg.DBGetter,
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
			w.cfg.Logger.Errorf("failed to remove history file: %v", err)
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
	controllerDB := currentDB
	currentNamespace := "controller"

	close(done)

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

		line.SetPrompt("repl (" + currentNamespace + ")> ")
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
		case ".exit":
			return worker.ErrTerminateAgent
		case ".help":
			fmt.Fprintf(w.cfg.Stdout, helpText)
			continue
		case ".switch":
			if len(args) != 2 {
				fmt.Fprintln(w.cfg.Stderr, "usage: .switch <name>")
				continue
			}

			argName := args[1]
			if argName == "controller" {
				currentDB = controllerDB
				currentNamespace = argName
				continue
			}
			parts := strings.Split(argName, "-")
			if len(parts) != 2 {
				fmt.Fprintln(w.cfg.Stderr, "invalid namespace name")
				continue
			} else if parts[0] != "model" {
				fmt.Fprintln(w.cfg.Stderr, "invalid model namespace name")
				continue
			}
			name := parts[1]

			var uuid string
			if err := controllerDB.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
				row := tx.QueryRowContext(ctx, "SELECT uuid FROM model WHERE name=?", name)
				if err := row.Scan(&uuid); err != nil {
					return err
				}
				return nil
			}); errors.Is(err, sql.ErrNoRows) {
				fmt.Fprintf(w.cfg.Stderr, "model %q not found\n", name)
				continue
			} else if err != nil {
				fmt.Fprintf(w.cfg.Stderr, "failed to select %q database: %v\n", name, err)
				continue
			}

			currentDB, err = w.dbGetter.GetDB(uuid)
			if err != nil {
				fmt.Fprintf(w.cfg.Stderr, "failed to switch to namespace %q: %v\n", name, err)
				continue
			}
			currentNamespace = argName

		case ".models":
			if err := w.executeQuery(ctx, controllerDB, "SELECT uuid, name FROM model;"); err != nil {
				w.cfg.Logger.Errorf("failed to execute query: %v", err)
			}

		default:
			if err := w.executeQuery(ctx, currentDB, input); err != nil {
				w.cfg.Logger.Errorf("failed to execute query: %v", err)
			}
		}
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

		var sb strings.Builder
		writer := tabwriter.NewWriter(&sb, 0, 8, 1, '\t', 0)
		for _, col := range columns {
			fmt.Fprintf(writer, "%s\t", col)
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

  .exit              Exit the REPL.
  .help              Show this help message.
  .models            Show all models.
  .switch            Switch to a different model (or global database).

The global database can be accessed by using the '*' or 'global' keyword
when switching databases. 
`
