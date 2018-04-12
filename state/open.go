// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"runtime/pprof"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/mongo"
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
	if err := args.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating args")
	}

	session := args.MongoSession.Copy()
	if args.InitDatabaseFunc != nil {
		if err := args.InitDatabaseFunc(session, args.ControllerModelTag.Id(), nil); err != nil {
			return nil, errors.Trace(err)
		}
		logger.Debugf("mongodb initialised")
	}

	return &Controller{
		clock:                  args.Clock,
		controllerTag:          args.ControllerTag,
		controllerModelTag:     args.ControllerModelTag,
		session:                session,
		newPolicy:              args.NewPolicy,
		runTransactionObserver: args.RunTransactionObserver,
	}, nil
}

// Open connects to the server with the given parameters, waits for it
// to be initialized, and returns a new State representing the model
// connected to.
//
// Open returns unauthorizedError if access is unauthorized.
func Open(args OpenParams) (*State, error) {
	if err := args.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating args")
	}
	session := args.MongoSession.Copy()
	st, err := open(
		args.ControllerModelTag,
		session,
		args.InitDatabaseFunc,
		nil,
		args.NewPolicy,
		args.Clock,
		args.RunTransactionObserver,
	)
	if err != nil {
		session.Close()
		return nil, errors.Trace(err)
	}
	if _, err := st.Model(); err != nil {
		if err := st.Close(); err != nil {
			logger.Errorf("closing State for %s: %v", args.ControllerModelTag, err)
		}
		return nil, mongo.MaybeUnauthorizedf(err, "cannot read model %s", args.ControllerModelTag.Id())
	}

	// State should only be Opened on behalf of a controller environ; all
	// other *States must be obtained via StatePool.
	if err := st.start(args.ControllerTag, nil); err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
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

	db := &database{
		raw:                    session.DB(jujuDB),
		schema:                 allCollections(),
		modelUUID:              modelTag.Id(),
		runTransactionObserver: runTransactionObserver,
		clock: clock,
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

	if st.workers != nil {
		if err := worker.Stop(st.workers); err != nil {
			return errors.Annotatef(err, "failed to stop workers")
		}
	}
	st.session.Close()
	logger.Debugf("closed state without error")
	// Remove the reference.
	profileTracker.Remove(st)
	return nil
}
