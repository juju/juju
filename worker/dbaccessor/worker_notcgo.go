// +build !cgo

// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
)

type unsupportedApp struct{}

// NewApp creates a new SQLite application.
func NewApp(dataDir string, options ...Option) (DBApp, error) {
	return &unsupportedApp{}, errors.NotSupportedf("db")
}

// Open the dqlite database with the given name
func (a *unsupportedApp) Open(ctx context.Context, databaseName string) (*sql.DB, error) {
	return nil, errors.NotSupportedf("db")
}

// Ready can be used to wait for a node to complete some initial tasks that are
// initiated at startup.
func (a *unsupportedApp) Ready(ctx context.Context) error {
	return errors.NotSupportedf("db")
}

// Handover transfers all responsibilities for this node (such has leadership
// and voting rights) to another node, if one is available.
//
// This method should always be called before invoking Close(), in order to
// gracefully shutdown a node.
func (a *unsupportedApp) Handover(ctx context.Context) error {
	return errors.NotSupportedf("db")
}

// ID returns the dqlite ID of this application node.
func (a *unsupportedApp) ID() uint64 {
	return 0
}

// Close the application node, releasing all resources it created.
func (a *unsupportedApp) Close() error {
	return errors.NotSupportedf("db")
}

// GetREPL returns a Repl worker from the underlying DB.
func (a *unsupportedApp) GetREPL(dbGetter DBGetter) (REPL, error) {
	return nil, errors.NotSupportedf("repl")
}
