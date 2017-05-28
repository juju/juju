// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"runtime/debug"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/featureflag"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/mongo"
)

type SessionCloser func()

func dontCloseAnything() {}

// Database exposes the mongodb capabilities that most of state should see.
type Database interface {

	// Copy returns a matching Database with its own session, and a
	// func that must be called when the Database is no longer needed.
	//
	// GetCollection and TransactionRunner results from the resulting Database
	// will all share a session; this does not absolve you of responsibility
	// for calling those collections' closers.
	Copy() (Database, SessionCloser)

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

	// GetCollecitonFor returns the named Collection, scoped for the
	// model specified. As for GetCollection, a closer is also returned.
	GetCollectionFor(modelUUID, name string) (mongo.Collection, SessionCloser)

	// GetRawCollection returns the named mgo Collection. As no
	// automatic model filtering is performed by the returned
	// collection it should be rarely used. GetCollection() should be
	// used in almost all cases.
	GetRawCollection(name string) (*mgo.Collection, SessionCloser)

	// TransactionRunner() returns a runner responsible for making changes to
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

	// RunTransaction is a convenience method for running a single
	// transaction for the model specified.
	RunTransactionFor(modelUUID string, ops []txn.Op) error

	// RunRawTransaction is a convenience method that will run a
	// single transaction using a "raw" transaction runner that won't
	// perform model filtering.
	RunRawTransaction(ops []txn.Op) error

	// Run is a convenience method running a transaction using a
	// transaction building function.
	Run(transactions jujutxn.TransactionSource) error

	// RunFor is like Run but runs the transaction for the model specified.
	RunFor(modelUUID string, transactions jujutxn.TransactionSource) error

	// Schema returns the schema used to load the database. The returned schema
	// is not a copy and must not be modified.
	Schema() collectionSchema
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
	db, closer := db.Copy()
	defer closer()

	buildTxn := func(int) ([]txn.Op, error) {
		ops, err := change.Prepare(db)
		if errors.Cause(err) == ErrChangeComplete {
			return nil, jujutxn.ErrNoOperations
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		return ops, nil
	}

	runner, closer := db.TransactionRunner()
	defer closer()
	if err := runner.Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// collectionInfo describes important features of a collection.
type collectionInfo struct {

	// explicitCreate, if non-nil, will cause the collection to be explicitly
	// Create~d (with the given value) before ensuring indexes.
	explicitCreate *mgo.CollectionInfo

	// indexes listed here will be EnsureIndex~ed before state is opened.
	indexes []mgo.Index

	// global collections will not have model filtering applied. Non-
	// global collections will have both transactions and reads filtered by
	// relevant model uuid.
	global bool

	// rawAccess collections can be safely accessed as a mongo.WriteCollection.
	// Direct database access to txn-aware collections is strongly discouraged:
	// merely writing directly to a field makes it impossible to use that field
	// with mgo/txn; in the worst case, document deletion can destroy critical
	// parts of the state distributed by mgo/txn, causing different runners to
	// choose different global transaction orderings; this can in turn cause
	// operations to be skipped.
	//
	// Short explanation follows: two different runners pick different -- but
	// overlapping -- "next" transactions; they each pick the same txn-id; the
	// first runner writes to an overlapping document and records the txn-id;
	// and then the second runner inspects that document, sees that the chosen
	// txn-id has already been applied, and <splat> skips that operation.
	//
	// Goodbye consistency. So please don't mix txn and non-txn writes without
	// very careful analysis; and then, please, just don't do it anyway. If you
	// need raw mgo, use a rawAccess collection.
	rawAccess bool
}

// collectionSchema defines the set of collections used in juju.
type collectionSchema map[string]collectionInfo

// Create causes all recorded collections to be created and indexed as specified
func (schema collectionSchema) Create(
	db *mgo.Database,
	settings *controller.Config,
) error {
	for name, info := range schema {
		rawCollection := db.C(name)
		if spec := info.explicitCreate; spec != nil {
			// We allow the max txn log collection size to be overridden by the user.
			if name == txnLogC && settings != nil {
				maxSize := settings.MaxTxnLogSizeMB()
				if maxSize > 0 {
					logger.Infof("overriding max txn log collection size: %dM", maxSize)
					spec.MaxBytes = maxSize * 1024 * 1024
				}
			}
			if err := createCollection(rawCollection, spec); err != nil {
				message := fmt.Sprintf("cannot create collection %q", name)
				return maybeUnauthorized(err, message)
			}
		}
		for _, index := range info.indexes {
			if err := rawCollection.EnsureIndex(index); err != nil {
				return maybeUnauthorized(err, "cannot create index")
			}
		}
	}
	return nil
}

// createCollection swallows collection-already-exists errors.
func createCollection(raw *mgo.Collection, spec *mgo.CollectionInfo) error {
	err := raw.Create(spec)
	// The lack of error code for this error was reported upstream:
	//     https://jira.mongodb.org/browse/SERVER-6992
	if err == nil || err.Error() == "collection already exists" {
		return nil
	}
	return err
}

// database implements Database.
type database struct {

	// raw is the underlying mgo Database.
	raw *mgo.Database

	// schema specifies how the various collections must be handled.
	schema collectionSchema

	// modelUUID is used to automatically filter queries and operations on
	// certain collections (as defined in .schema).
	modelUUID string

	// runner exists for testing purposes; if non-nil, the result of
	// TransactionRunner will always ultimately use this value to run
	// all transactions. Setting it renders the database goroutine-unsafe.
	runner jujutxn.Runner

	// ownSession is used to avoid copying additional sessions in a database
	// resulting from Copy.
	ownSession bool

	// runTransactionObserver is passed on to txn.TransactionRunner, to be
	// invoked after calls to Run and RunTransaction.
	runTransactionObserver RunTransactionObserverFunc
}

// RunTransactionObserverFunc is the type of a function to be called
// after an mgo/txn transaction is run.
type RunTransactionObserverFunc func(dbName, modelUUID string, ops []txn.Op, err error)

func (db *database) copySession(modelUUID string) (*database, SessionCloser) {
	session := db.raw.Session.Copy()
	return &database{
		raw:        db.raw.With(session),
		schema:     db.schema,
		modelUUID:  modelUUID,
		runner:     db.runner,
		ownSession: true,
	}, session.Close
}

// Copy is part of the Database interface.
func (db *database) Copy() (Database, SessionCloser) {
	return db.copySession(db.modelUUID)
}

// CopyForModel is part of the Database interface.
func (db *database) CopyForModel(modelUUID string) (Database, SessionCloser) {
	return db.copySession(modelUUID)
}

// GetCollection is part of the Database interface.
func (db *database) GetCollection(name string) (collection mongo.Collection, closer SessionCloser) {
	info, found := db.schema[name]
	if !found {
		logger.Errorf("using unknown collection %q", name)
		if featureflag.Enabled(feature.DeveloperMode) {
			logger.Errorf("from %s", string(debug.Stack()))
		}
	}

	// Copy session if necessary.
	if db.ownSession {
		collection = mongo.WrapCollection(db.raw.C(name))
		closer = dontCloseAnything
	} else {
		collection, closer = mongo.CollectionFromName(db.raw, name)
	}

	// Apply model filtering.
	if !info.global {
		collection = &modelStateCollection{
			WriteCollection: collection.Writeable(),
			modelUUID:       db.modelUUID,
		}
	}

	// Prevent layer-breaking.
	if !info.rawAccess {
		// TODO(fwereade): it would be nice to tweak the mongo.Collection
		// interface a bit to drop Writeable in this situation, but it's
		// not convenient yet.
	}
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
	collection, closer := db.GetCollection(name)
	return collection.Writeable().Underlying(), closer
}

// TransactionRunner is part of the Database interface.
func (db *database) TransactionRunner() (runner jujutxn.Runner, closer SessionCloser) {
	runner = db.runner
	closer = dontCloseAnything
	if runner == nil {
		raw := db.raw
		if !db.ownSession {
			session := raw.Session.Copy()
			raw = raw.With(session)
			closer = session.Close
		}
		var observer func([]txn.Op, error)
		if db.runTransactionObserver != nil {
			observer = func(ops []txn.Op, err error) {
				db.runTransactionObserver(
					db.raw.Name, db.modelUUID,
					ops, err,
				)
			}
		}
		params := jujutxn.RunnerParams{
			Database:               raw,
			RunTransactionObserver: observer,
		}
		runner = jujutxn.NewRunner(params)
	}
	return &multiModelRunner{
		rawRunner: runner,
		modelUUID: db.modelUUID,
		schema:    db.schema,
	}, closer
}

// RunTransaction is part of the Database interface.
func (db *database) RunTransaction(ops []txn.Op) error {
	runner, closer := db.TransactionRunner()
	defer closer()
	return runner.RunTransaction(ops)
}

// RunTransactionFor is part of the Database interface.
func (db *database) RunTransactionFor(modelUUID string, ops []txn.Op) error {
	newDB, dbcloser := db.CopyForModel(modelUUID)
	defer dbcloser()
	runner, closer := newDB.TransactionRunner()
	defer closer()
	return runner.RunTransaction(ops)
}

// RunRawTransaction is part of the Database interface.
func (db *database) RunRawTransaction(ops []txn.Op) error {
	runner, closer := db.TransactionRunner()
	defer closer()
	if multiRunner, ok := runner.(*multiModelRunner); ok {
		runner = multiRunner.rawRunner
	}
	return runner.RunTransaction(ops)
}

// Run is part of the Database interface.
func (db *database) Run(transactions jujutxn.TransactionSource) error {
	runner, closer := db.TransactionRunner()
	defer closer()
	return runner.Run(transactions)
}

// RunFor is part of the Database interface.
func (db *database) RunFor(modelUUID string, transactions jujutxn.TransactionSource) error {
	newDB, dbcloser := db.CopyForModel(modelUUID)
	defer dbcloser()
	runner, closer := newDB.TransactionRunner()
	defer closer()
	return runner.Run(transactions)
}

// Schema is part of the Database interface.
func (db *database) Schema() collectionSchema {
	return db.schema
}
