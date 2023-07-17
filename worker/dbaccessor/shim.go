// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/dqlite"
)

// Client describes a client that speaks the Dqlite wire protocol,
// and can retrieve cluster information.
type Client interface {
	// Cluster returns the current state of the Dqlite cluster.
	Cluster(context.Context) ([]dqlite.NodeInfo, error)
	// Leader returns information about the current leader, if any.
	Leader(ctx context.Context) (*dqlite.NodeInfo, error)
}

// DBApp describes methods of a Dqlite database application,
// required to run this host as a Dqlite node.
type DBApp interface {
	// Open the dqlite database with the given name
	Open(context.Context, string) (*sql.DB, error)

	// Ready can be used to wait for a node to complete tasks that
	// are initiated at startup. For example a new node will attempt
	// to join the cluster, a restarted node will check if it should
	// assume some particular role, etc.
	//
	// If this method returns without error it means that those initial
	// tasks have succeeded and follow-up operations like Open() are more
	// likely to succeed quickly.
	Ready(context.Context) error

	// Client returns a client that can be used
	// to interrogate the Dqlite cluster.
	Client(ctx context.Context) (Client, error)

	// Handover transfers all responsibilities for this node (such has
	// leadership and voting rights) to another node, if one is available.
	//
	// This method should always be called before invoking Close(),
	// in order to gracefully shut down a node.
	Handover(context.Context) error

	// ID returns the dqlite ID of this application node.
	ID() uint64

	// Address returns the bind address of this application node.
	Address() string

	// Close the application node, releasing all resources it created.
	Close() error
}

// dbApp wraps a Dqlite App reference, so that we can shim out Client.
type dbApp struct {
	*app.App
}

// Client implements DBApp by returning a Client indirection.
func (a *dbApp) Client(ctx context.Context) (Client, error) {
	c, err := a.App.Client(ctx)
	return c, errors.Trace(err)
}

// NewApp creates a new DQlite application.
func NewApp(dataDir string, options ...app.Option) (DBApp, error) {
	dqliteApp, err := app.New(dataDir, options...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return WrapApp(dqliteApp), nil
}

// WrapApp wraps a Dqlite App reference, so that we can shim out Client.
func WrapApp(dqliteApp *app.App) DBApp {
	return &dbApp{dqliteApp}
}
