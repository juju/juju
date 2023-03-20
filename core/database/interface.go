// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

// DBGetter describes the ability to supply a TrackedDB reference for a
// particular database.
type DBGetter interface {
	// GetDB returns a TrackedDB reference for the dqlite-backed database that
	// contains the data for the specified namespace.
	// A NotFound error is returned if the worker is unaware of the requested
	// DB.
	GetDB(namespace string) (TrackedDB, error)
}

const (
	// ControllerNS is the namespace for the controller database.
	ControllerNS = "controller"
)
