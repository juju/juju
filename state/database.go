// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/mongo"
)

type SessionCloser func()

// Database exposes the mongodb capabilities that most of state should see.
type Database interface {

	// Copy returns a matching Database with its own session, and a
	// func that must be called when the Database is no longer needed.
	//
	// GetCollection and TransactionRunner results from the resulting Database
	// will all share a session; this does not absolve you of responsibility
	// for calling those collections' closers.
	Copy() (Database, SessionCloser)

	// CopyRaw returns a matching Database with its own session, and the
	// session, which must be closed when the Database is no longer needed.
	CopyRaw() (Database, *mgo.Session)

	// CopyForModel returns a matching Database with its own session and
	// its own modelUUID and a func that must be called when the Database is no
	// longer needed.
	//
	// Same warnings apply for CopyForModel than for Copy.
	CopyForModel(modelUUID string) (Database, SessionCloser)

	// GetCollection returns the named Collection, and a func that must be
	// called when the Collection is no longer needed. The returned Collection
	// might or might not have its own session, depending on the Database; the
	// closer must always be called regardless.
	//
	// If the schema specifies model-filtering for the named collection,
	// the returned collection will automatically filter queries; for details,
	// see modelStateCollection.
	GetCollection(name string) (mongo.Collection, SessionCloser)

	// GetCollectionFor returns the named collection, scoped for the
	// model specified. As for GetCollection, a closer is also returned.
	GetCollectionFor(modelUUID, name string) (mongo.Collection, SessionCloser)

	// GetRawCollection returns the named mgo Collection. As no
	// automatic model filtering is performed by the returned
	// collection it should be rarely used. GetCollection() should be
	// used in almost all cases.
	GetRawCollection(name string) (*mgo.Collection, SessionCloser)

	// TransactionRunner returns a runner responsible for making changes to
	// the database, and a func that must be called when the runner is no longer
	// needed. The returned Runner might or might not have its own session,
	// depending on the Database; the closer must always be called regardless.
	//
	// It will reject transactions that reference raw-access (or unknown)
	// collections; it will automatically rewrite operations that reference
	// non-global collections; and it will ensure that non-global documents can
	// only be inserted while the corresponding model is still Alive.
	TransactionRunner() (jujutxn.Runner, SessionCloser)

	// RunTransaction is a convenience method for running a single
	// transaction.
	RunTransaction(ops []txn.Op) error

	// RunTransactionFor is a convenience method for running a single
	// transaction for the model specified.
	RunTransactionFor(modelUUID string, ops []txn.Op) error

	// RunRawTransaction is a convenience method that will run a
	// single transaction using a "raw" transaction runner that won't
	// perform model filtering.
	RunRawTransaction(ops []txn.Op) error

	// Run is a convenience method running a transaction using a
	// transaction building function.
	Run(transactions jujutxn.TransactionSource) error

	// Run is a convenience method running a transaction using a
	// transaction building function using a "raw" transaction runner
	// that won't perform model filtering.
	RunRaw(transactions jujutxn.TransactionSource) error

	// Schema returns the schema used to load the database. The returned schema
	// is not a copy and must not be modified.
	Schema() CollectionSchema
}

// Change represents any mgo/txn-representable change to a Database.
type Change interface {

	// Prepare ensures that db is in a valid base state for applying
	// the change, and returns mgo/txn operations that will fail any
	// enclosing transaction if the state has materially changed; or
	// returns an error.
	Prepare(db Database) ([]txn.Op, error)
}

// ErrChangeComplete can be returned from Prepare to finish an Apply
// attempt and report success without taking any further action.
var ErrChangeComplete = errors.New("change complete")

// Apply runs the supplied Change against the supplied Database. If it
// returns no error, the change succeeded.
func Apply(db Database, change Change) error {
	return nil
}

// CollectionInfo describes important features of a collection.
type CollectionInfo struct {
}

// CollectionSchema defines the set of collections used in juju.
type CollectionSchema map[string]CollectionInfo

// Create causes all recorded collections to be created and indexed as specified
func (schema CollectionSchema) Create(
	db *mgo.Database,
	settings *controller.Config,
) error {
	return nil
}

// database implements Database.
type database struct {
}

// Copy is part of the Database interface.
func (db *database) Copy() (Database, SessionCloser) {
	return &database{}, func() {}
}

// CopyRaw is part of the Database interface.
func (db *database) CopyRaw() (Database, *mgo.Session) {
	return &database{}, nil
}

// CopyForModel is part of the Database interface.
func (db *database) CopyForModel(modelUUID string) (Database, SessionCloser) {
	return &database{}, func() {}
}

// GetCollection is part of the Database interface.
func (db *database) GetCollection(name string) (collection mongo.Collection, closer SessionCloser) {
	collection = &modelStateCollection{}
	closer = func() {}
	return collection, closer
}

// GetCollectionFor is part of the Database interface.
func (db *database) GetCollectionFor(modelUUID, name string) (mongo.Collection, SessionCloser) {
	newDb, dbcloser := db.CopyForModel(modelUUID)
	collection, closer := newDb.GetCollection(name)
	return collection, func() {
		closer()
		dbcloser()
	}
}

// GetRawCollection is part of the Database interface.
func (db *database) GetRawCollection(name string) (*mgo.Collection, SessionCloser) {
	return nil, func() {}
}

// TransactionRunner is part of the Database interface.
func (db *database) TransactionRunner() (runner jujutxn.Runner, closer SessionCloser) {
	return &multiModelRunner{}, func() {}
}

// RunTransaction is part of the Database interface.
func (db *database) RunTransaction(ops []txn.Op) error {
	return nil
}

// RunTransactionFor is part of the Database interface.
func (db *database) RunTransactionFor(modelUUID string, ops []txn.Op) error {
	return nil
}

// RunRawTransaction is part of the Database interface.
func (db *database) RunRawTransaction(ops []txn.Op) error {
	return nil
}

// Run is part of the Database interface.
func (db *database) Run(transactions jujutxn.TransactionSource) error {
	return nil
}

// RunRaw is part of the Database interface.
func (db *database) RunRaw(transactions jujutxn.TransactionSource) error {
	return nil
}

// Schema is part of the Database interface.
func (db *database) Schema() CollectionSchema {
	return CollectionSchema{}
}
