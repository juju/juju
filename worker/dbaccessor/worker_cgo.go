// +build cgo

// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	dqApp "github.com/canonical/go-dqlite/app"
	"github.com/canonical/go-dqlite/driver"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/mattn/go-sqlite3"
)

type dqliteApp struct {
	dataDir   string
	dqliteApp *dqApp.App
	clock     clock.Clock
	logger    Logger
}

// NewApp creates a new DQlite application.
func NewApp(dataDir string, options ...Option) (DBApp, error) {
	opts := newOptions()
	for _, option := range options {
		option(opts)
	}

	app, err := dqApp.New(dataDir,
		dqApp.WithAddress(opts.Address),
		dqApp.WithCluster(opts.Cluster),
		dqApp.WithTLS(opts.TLS.Listen, opts.TLS.Dial),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &dqliteApp{
		dataDir:   dataDir,
		dqliteApp: app,
		clock:     opts.Clock,
		logger:    opts.Logger,
	}, nil
}

// Open the dqlite database with the given name
func (a *dqliteApp) Open(ctx context.Context, databaseName string) (*sql.DB, error) {
	return a.dqliteApp.Open(ctx, databaseName)
}

// Ready can be used to wait for a node to complete some initial tasks that are
// initiated at startup. For example a brand new node will attempt to join the
// cluster, a restarted node will check if it should assume some particular
// role, etc.
//
// If this method returns without error it means that those initial tasks have
// succeeded and follow-up operations like Open() are more likely to succeeed
// quickly.
func (a *dqliteApp) Ready(ctx context.Context) error {
	return a.dqliteApp.Ready(ctx)
}

// Handover transfers all responsibilities for this node (such has leadership
// and voting rights) to another node, if one is available.
//
// This method should always be called before invoking Close(), in order to
// gracefully shutdown a node.
func (a *dqliteApp) Handover(ctx context.Context) error {
	return a.dqliteApp.Handover(ctx)
}

// ID returns the dqlite ID of this application node.
func (a *dqliteApp) ID() uint64 {
	return a.dqliteApp.ID()
}

// Close the application node, releasing all resources it created.
func (a *dqliteApp) Close() error {
	return a.dqliteApp.Close()
}

// Repl returns a Repl worker from the underlying DB.
func (a *dqliteApp) Repl(dbGetter DBGetter) (REPL, error) {
	replSocket := filepath.Join(a.dataDir, "juju.sock")
	_ = os.Remove(replSocket)

	return newREPL(replSocket, dbGetter, isRetriableError, a.clock, a.logger)
}

// isRetriableError returns true if the given error might be transient and the
// interaction can be safely retried.
func isRetriableError(err error) bool {
	err = errors.Cause(err)
	if err == nil {
		return false
	}

	if err, ok := err.(driver.Error); ok && err.Code == driver.ErrBusy {
		return true
	}

	if err == sqlite3.ErrLocked || err == sqlite3.ErrBusy {
		return true
	}

	if strings.Contains(err.Error(), "database is locked") {
		return true
	}

	if strings.Contains(err.Error(), "cannot start a transaction within a transaction") {
		return true
	}

	if strings.Contains(err.Error(), "bad connection") {
		return true
	}

	if strings.Contains(err.Error(), "checkpoint in progress") {
		return true
	}

	return false
}
