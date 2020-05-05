// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"runtime/pprof"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/names/v4"
	"github.com/juju/txn"
	"github.com/juju/worker/v2"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/feature"
)

// Register the state tracker as a new profile.
var profileTracker = pprof.NewProfile("juju/state/tracker")

// OpenParams contains the parameters for opening the state database.
type OpenParams struct {
	// Clock is the clock used for time-related operations.
	Clock clock.Clock

	// ControllerTag is the tag of the controller.
	ControllerTag names.ControllerTag

	// ControllerModelTag is the tag of the controller model.
	ControllerModelTag names.ModelTag

	// MongoSession is the mgo.Session to use for storing and
	// accessing state data. The caller remains responsible
	// for closing this session; Open will copy it.
	MongoSession *mgo.Session

	// NewPolicy, if non-nil, returns a policy which will be used to
	// validate and modify behaviour of certain operations in state.
	NewPolicy NewPolicyFunc

	// RunTransactionObserver, if non-nil, is a function that will
	// be called after mgo/txn transactions are run, successfully
	// or not.
	RunTransactionObserver RunTransactionObserverFunc

	// InitDatabaseFunc, if non-nil, is a function that will be called
	// just after the state database is opened.
	InitDatabaseFunc InitDatabaseFunc
}

// Validate validates the OpenParams.
func (p OpenParams) Validate() error {
	if p.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if p.ControllerTag == (names.ControllerTag{}) {
		return errors.NotValidf("empty ControllerTag")
	}
	if p.ControllerModelTag == (names.ModelTag{}) {
		return errors.NotValidf("empty ControllerModelTag")
	}
	if p.MongoSession == nil {
		return errors.NotValidf("nil MongoSession")
	}
	return nil
}

// OpenController connects to the server with the given parameters, waits for it
// to be initialized, and returns a new Controller instance.
//
// OpenController returns unauthorizedError if access is unauthorized.
func OpenController(args OpenParams) (*Controller, error) {
	pool, err := OpenStatePool(args)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &Controller{
		pool:     pool,
		ownsPool: true,
	}, nil
}

func open(
	controllerModelTag names.ModelTag,
	session *mgo.Session,
	initDatabase InitDatabaseFunc,
	controllerConfig *controller.Config,
	newPolicy NewPolicyFunc,
	clock clock.Clock,
	runTransactionObserver RunTransactionObserverFunc,
) (*State, error) {
	st, err := newState(controllerModelTag, controllerModelTag, session, newPolicy, clock, runTransactionObserver)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if initDatabase != nil {
		if err := initDatabase(session, controllerModelTag.Id(), controllerConfig); err != nil {
			session.Close()
			return nil, errors.Trace(err)
		}
		logger.Debugf("mongodb initialised")
	}

	return st, nil
}

// newState creates an incomplete *State, with no running workers or
// controllerTag. You must start() the returned *State before it will
// function correctly.
// modelTag is used to filter all queries and transactions.
//
// newState takes responsibility for the supplied *mgo.Session, and will
// close it if it cannot be returned under the aegis of a *State.
func newState(
	modelTag, controllerModelTag names.ModelTag,
	session *mgo.Session,
	newPolicy NewPolicyFunc,
	clock clock.Clock,
	runTransactionObserver RunTransactionObserverFunc,
) (_ *State, err error) {

	defer func() {
		if err != nil {
			session.Close()
		}
	}()

	mongodb := session.DB(jujuDB)
	sstxn := featureflag.Enabled(feature.MongoDbSSTXN)
	if sstxn {
		if !txn.SupportsServerSideTransactions(mongodb) {
			logger.Warningf("User requested server-side transactions, but they are not supported.\n"+
				" Falling back to client-side transactions.\n"+
				" Consider using the '%s' feature flag", feature.MongoDbSnap)
			sstxn = false
		} else {
			logger.Infof("using server-side transactions")
		}
	} else {
		logger.Infof("using client-side transactions")
	}
	db := &database{
		raw:                    mongodb,
		schema:                 allCollections(),
		modelUUID:              modelTag.Id(),
		runTransactionObserver: runTransactionObserver,
		serverSideTransactions: sstxn,
		clock:                  clock,
	}

	// Create State.
	st := &State{
		stateClock:             clock,
		modelTag:               modelTag,
		controllerModelTag:     controllerModelTag,
		session:                session,
		database:               db,
		newPolicy:              newPolicy,
		runTransactionObserver: runTransactionObserver,
	}
	if newPolicy != nil {
		st.policy = newPolicy(st)
	}
	// Record this State instance with the global tracker.
	profileTracker.Add(st, 1)
	return st, nil
}

// Close the connection to the database.
func (st *State) Close() (err error) {
	defer errors.DeferredAnnotatef(&err, "closing state failed")
	if err := st.stopWorkers(); err != nil {
		return errors.Trace(err)
	}
	st.session.Close()
	logger.Debugf("closed state without error")
	// Remove the reference.
	profileTracker.Remove(st)
	return nil
}

func (st *State) stopWorkers() (err error) {
	if st.workers != nil {
		if err := worker.Stop(st.workers); err != nil {
			return errors.Annotatef(err, "failed to stop workers")
		}
	}
	return nil
}
