// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbreplaccessor

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
)

// DBApp describes methods of a Dqlite database application,
// required to run this host as a Dqlite node.
type DBApp interface {
	// Open the dqlite database with the given name
	Open(context.Context, string) (*sql.DB, error)
}

// NewAppFunc creates a new Dqlite application.
type NewAppFunc func(driverName string) (DBApp, error)

// dbApp wraps a Dqlite App reference, so that we can shim out Client.
type dbApp struct {
	driverName string
}

// NewApp creates a new type for opening dqlite databases for a given driver
// name.
func NewApp(driverName string) (DBApp, error) {
	return &dbApp{driverName: driverName}, nil
}

func (a *dbApp) Open(ctx context.Context, name string) (*sql.DB, error) {
	db, err := sql.Open(a.driverName, name)
	if err != nil {
		return nil, errors.Annotatef(err, "opening dqlite database for namespace %q", name)
	}

	return db, nil
}
