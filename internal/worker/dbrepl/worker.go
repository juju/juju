// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbrepl

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/peterh/liner"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/database"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/database/client"
	"github.com/juju/juju/internal/database/dqlite"
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

// dbRequest is used to pass requests for TrackedDB
// instances into the worker loop.
type dbRequest struct {
	namespace string
	done      chan error
}

// makeDBGetRequest creates a new TrackedDB request for the input namespace.
func makeDBGetRequest(namespace string) dbRequest {
	return dbRequest{
		namespace: namespace,
		done:      make(chan error),
	}
}

// WorkerConfig encapsulates the configuration options for the
// dbaccessor worker.
type WorkerConfig struct {
	DBGetter coredatabase.DBGetter
	Clock    clock.Clock
	Logger   logger.Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.DBGetter == nil {
		return errors.NotValidf("missing DBGetter")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
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

	loggo.GetLogger("***").Criticalf("dbReplWorker loop")

	line := liner.NewLiner()
	defer line.Close()

	db, err := w.dbGetter.GetDB(database.ControllerNS)
	if err != nil {
		return errors.Annotate(err, "failed to get db")
	}

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		default:
		}

		input, err := line.Prompt("repl> ")
		if err != nil {
			return errors.Annotate(err, "failed to read input")
		}

		err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, input)
			if err != nil {
				return err
			}

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
			return err
		})
		if err != nil {
			w.cfg.Logger.Errorf("failed to execute query: %v", err)
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
