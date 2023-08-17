//go:build !dqlite

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package app

import (
	"context"
	"crypto/tls"
	"database/sql"
	"path/filepath"

	"github.com/juju/errors"
	_ "github.com/mattn/go-sqlite3"

	"github.com/juju/juju/database/client"
)

// Option can be used to tweak app parameters.
type Option func()

// WithAddress sets the network address of the application node.
//
// Other application nodes must be able to connect to this application node
// using the given address.
//
// If the application node is not the first one in the cluster, the address
// must match the value that was passed to the App.Add() method upon
// registration.
//
// If not given the first non-loopback IP address of any of the system network
// interfaces will be used, with port 9000.
//
// The address must be stable across application restarts.
func WithAddress(address string) Option {
	return func() {}
}

// WithCluster must be used when starting a newly added application node for
// the first time.
//
// It should contain the addresses of one or more applications nodes which are
// already part of the cluster.
func WithCluster(cluster []string) Option {
	return func() {}
}

// WithTLS enables TLS encryption of network traffic.
//
// The "listen" parameter must hold the TLS configuration to use when accepting
// incoming connections clients or application nodes.
//
// The "dial" parameter must hold the TLS configuration to use when
// establishing outgoing connections to other application nodes.
func WithTLS(listen *tls.Config, dial *tls.Config) Option {
	return func() {}
}

// WithLogFunc sets a custom log function.
func WithLogFunc(log client.LogFunc) Option {
	return func() {}
}

// WithTracing will emit a log message at the given level every time a
// statement gets executed.
func WithTracing(level client.LogLevel) Option {
	return func() {}
}

// App is a high-level helper for initializing a typical dqlite-based Go
// application.
//
// It takes care of starting a dqlite node and registering a dqlite Go SQL
// driver.
type App struct {
	dir string
}

// New creates a new application node.
func New(dir string, options ...Option) (*App, error) {
	return &App{dir: dir}, nil
}

// Ready can be used to wait for a node to complete tasks that
// are initiated at startup. For example a new node will attempt
// to join the cluster, a restarted node will check if it should
// assume some particular role, etc.
//
// If this method returns without error it means that those initial
// tasks have succeeded and follow-up operations like Open() are more
// likely to succeed quickly.
func (*App) Ready(_ context.Context) error {
	return nil
}

// Open the dqlite database with the given name
func (a *App) Open(_ context.Context, name string) (*sql.DB, error) {
	path := name
	if name != ":memory:" {
		path = filepath.Join(a.dir, name)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return db, nil
}

// Handover transfers all responsibilities for this node (such has
// leadership and voting rights) to another node, if one is available.
//
// This method should always be called before invoking Close(),
// in order to gracefully shut down a node.
func (*App) Handover(context.Context) error {
	return nil
}

// ID returns the dqlite ID of this application node.
func (*App) ID() uint64 {
	return 1
}

func (*App) Client(context.Context) (*client.Client, error) {
	return &client.Client{}, nil
}

func (*App) Address() string {
	return ""
}

func (*App) Close() error {
	return nil
}
