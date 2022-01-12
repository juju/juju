// +build cgo

// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"

	dqApp "github.com/canonical/go-dqlite/app"
	"github.com/canonical/go-dqlite/driver"
	"github.com/juju/errors"
	"github.com/mattn/go-sqlite3"
)

type dqliteApp struct {
	dqliteApp *dqApp.App
}

// NewApp creates a new DQlite application.
func NewApp(dataDir string, options ...Option) (DBApp, error) {
	opts := newOptions()
	for _, option := range options {
		option(opts)
	}

	app, err := dqApp.New(dataDir, dqApp.WithAddress(opts.Address), dqApp.WithCluster(opts.Cluster))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &dqliteApp{
		dqliteApp: app,
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

func isDBAppError(err error) bool {
	if err, ok := err.(driver.Error); ok && err.Code == driver.ErrBusy {
		return true
	}

	if err == sqlite3.ErrLocked || err == sqlite3.ErrBusy {
		return true
	}
	return false
}
