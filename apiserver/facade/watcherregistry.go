// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facade

import "github.com/juju/worker/v4"

// WatcherRegistry holds all the watchers for a connection.
// It allows the registration of watchers that will be cleaned up when a
// connection terminates.
type WatcherRegistry interface {
	worker.Worker

	// Get returns the watcher for the given id, or nil if there is no such
	// watcher.
	Get(string) (worker.Worker, error)
	// Register registers the given watcher. It returns a unique identifier for the
	// watcher which can then be used in subsequent API requests to refer to the
	// watcher.
	Register(worker.Worker) (string, error)

	// RegisterNamed registers the given watcher. Callers must supply a unique
	// name for the given watcher. It is an error to try to register another
	// watcher with the same name as an already registered name.
	// It is also an error to supply a name that is an integer string, since that
	// collides with the auto-naming from Register.
	RegisterNamed(string, worker.Worker) error

	// Stop stops the resource with the given id and unregisters it.
	// It returns any error from the underlying Stop call.
	// It does not return an error if the resource has already
	// been unregistered.
	Stop(id string) error

	// Count returns the number of resources currently held.
	Count() int
}
