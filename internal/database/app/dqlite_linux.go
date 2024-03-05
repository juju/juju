//go:build dqlite && linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package app

import (
	"crypto/tls"
	"sync"

	"github.com/canonical/go-dqlite/app"
	"github.com/canonical/go-dqlite/client"
	"github.com/juju/errors"
)

// Option can be used to tweak app parameters.
type Option = app.Option

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
	return app.WithAddress(address)
}

// WithCluster must be used when starting a newly added application node for
// the first time.
//
// It should contain the addresses of one or more applications nodes which are
// already part of the cluster.
func WithCluster(cluster []string) Option {
	return app.WithCluster(cluster)
}

// WithTLS enables TLS encryption of network traffic.
//
// The "listen" parameter must hold the TLS configuration to use when accepting
// incoming connections clients or application nodes.
//
// The "dial" parameter must hold the TLS configuration to use when
// establishing outgoing connections to other application nodes.
func WithTLS(listen *tls.Config, dial *tls.Config) Option {
	return app.WithTLS(listen, dial)
}

// WithLogFunc sets a custom log function.
func WithLogFunc(log client.LogFunc) Option {
	return app.WithLogFunc(log)
}

// WithTracing will emit a log message at the given level every time a
// statement gets executed.
func WithTracing(level client.LogLevel) Option {
	return app.WithTracing(level)
}

// App is a high-level helper for initializing a typical dqlite-based Go
// application.
//
// It takes care of starting a dqlite node and registering a dqlite Go SQL
// driver.
type App struct {
	*app.App

	closer *onceError
}

// New creates a new application node.
func New(dir string, options ...Option) (*App, error) {
	app, err := app.New(dir, options...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &App{
		App:    app,
		closer: &onceError{},
	}, nil
}

// Close will close the application.
// This will close it exactly once. Any subsequent calls will return the same
// error.
func (a *App) Close() error {
	return a.closer.Do(func() error {
		return a.App.Close()
	})

}

// onceError is an object that will perform exactly one action.
type onceError struct {
	once  sync.Once
	mutex sync.Mutex
	err   error
}

// Do calls the function f if and only if Do is being called for the
// first time for this instance of Once. In other words, given
//
//	var once Once
//
// if once.Do(f) is called multiple times, only the first call will invoke f,
// even if f has a different value in each invocation. A new instance of
// Once is required for each function to execute.
//
// If f panics, Do considers it to have returned; future calls of Do return
// without calling f.
func (o *onceError) Do(f func() error) error {
	// Attempt to get the error without invoking f.
	o.mutex.Lock()
	if o.err != nil {
		o.mutex.Unlock()
		return o.err
	}

	// We need to invoke f at least once. We'll hold onto the lock until
	// we're done. This should ensure there isn't a race between the first
	// invocation of f and the return of the error.
	defer o.mutex.Unlock()

	// Do the work.
	o.once.Do(func() {
		o.err = f()
	})

	return o.err
}
