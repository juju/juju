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

// DBManager describes the ability to create and delete databases.
type DBManager interface {
	DBGetter

	// DeleteDB deletes the dqlite-backed database that contains the data for
	// the specified namespace.
	// There are currently a set of limitations on the namespaces that can be
	// deleted:
	//  - It's not possible to delete the controller database.
	//  - It currently doesn't support the actual deletion of the database
	//    just the removal of the worker. Deletion of the database will be
	//    handled once it's supported by dqlite.
	DeleteDB(namespace string) error
}

const (
	// ControllerNS is the namespace for the controller database.
	ControllerNS = "controller"
)
