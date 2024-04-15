// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"github.com/juju/errors"
)

const (
	// ControllerNS is the namespace for the controller database.
	ControllerNS = "controller"

	// ErrDBAccessorDying is used to indicate to *third parties* that the
	// db-accessor worker is dying, instead of catacomb.ErrDying, which is
	// unsuitable for propagating inter-worker.
	// This error indicates to consuming workers that their dependency has
	// become unmet and a restart by the dependency engine is imminent.
	ErrDBAccessorDying = errors.ConstError("db-accessor worker is dying")
)

// DBGetter describes the ability to supply a transaction runner
// for a particular database.
type DBGetter interface {
	// GetDB returns a TransactionRunner for the dqlite-backed database
	// that contains the data for the specified namespace.
	// A NotFound error is returned if the worker is unaware of the
	// requested DB.
	GetDB(namespace string) (TxnRunner, error)
}

// DBDeleter describes the ability to delete a database.
type DBDeleter interface {
	// DeleteDB deletes the dqlite-backed database that contains the data for
	// the specified namespace.
	// There are currently a set of limitations on the namespaces that can be
	// deleted:
	//  - It is not possible to delete the controller database.
	//  - It currently doesn't support the actual deletion of the database
	//    just the removal of the worker. Deletion of the database will be
	//    handled once it's supported by dqlite.
	DeleteDB(namespace string) error
}
