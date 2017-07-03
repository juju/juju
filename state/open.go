// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"runtime/pprof"
	"strings"

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

	// MongoInfo is the mongo.MongoInfo used for dialling the
	// Mongo connection.
	MongoInfo *mongo.MongoInfo

	// MongoDialOpts is the mongo.DialOpts used to control how
	// Mongo connections are made.
	MongoDialOpts mongo.DialOpts

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
	if p.MongoInfo == nil {
		return errors.NotValidf("nil MongoInfo")
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

	logger.Infof("opening controller state, mongo addresses: %q; entity %v",
		args.MongoInfo.Addrs, args.MongoInfo.Tag)
	logger.Debugf("dialing mongo")
	session, err := mongo.DialWithInfo(args.MongoInfo.Info, args.MongoDialOpts)
	if err != nil {
		return nil, maybeUnauthorized(err, "cannot connect to mongodb")
	}
	logger.Debugf("connection established")

	if err := mongodbLogin(session, args.MongoInfo); err != nil {
		session.Close()
		return nil, errors.Trace(err)
	}
	logger.Debugf("mongodb login successful")

	if args.InitDatabaseFunc != nil {
		if err := args.InitDatabaseFunc(session, args.ControllerModelTag.Id(), nil); err != nil {
			session.Close()
			return nil, errors.Trace(err)
		}
		logger.Debugf("mongodb initialised")
	}

	return &Controller{
		clock:                  args.Clock,
		controllerTag:          args.ControllerTag,
		controllerModelTag:     args.ControllerModelTag,
		mongoInfo:              args.MongoInfo,
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
	st, err := open(
		args.ControllerModelTag,
		args.MongoInfo,
		args.MongoDialOpts,
		args.InitDatabaseFunc,
		nil,
		args.NewPolicy,
		args.Clock,
		args.RunTransactionObserver,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, err := st.Model(); err != nil {
		if err := st.Close(); err != nil {
			logger.Errorf("closing State for %s: %v", args.ControllerModelTag, err)
		}
		return nil, maybeUnauthorized(err, fmt.Sprintf("cannot read model %s", args.ControllerModelTag.Id()))
	}

	// State should only be Opened on behalf of a controller environ; all
	// other *States should be created via ForModel.
	if err := st.start(args.ControllerTag); err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

func open(
	controllerModelTag names.ModelTag,
	info *mongo.MongoInfo, opts mongo.DialOpts,
	initDatabase InitDatabaseFunc,
	controllerConfig *controller.Config,
	newPolicy NewPolicyFunc,
	clock clock.Clock,
	runTransactionObserver RunTransactionObserverFunc,
) (*State, error) {
	logger.Infof("opening state, mongo addresses: %q; entity %v", info.Addrs, info.Tag)
	logger.Debugf("dialing mongo")
	session, err := mongo.DialWithInfo(info.Info, opts)
	if err != nil {
		return nil, maybeUnauthorized(err, "cannot connect to mongodb")
	}
	logger.Debugf("connection established")

	err = mongodbLogin(session, info)
	if err != nil {
		session.Close()
		return nil, errors.Trace(err)
	}
	logger.Debugf("mongodb login successful")

	st, err := newState(controllerModelTag, controllerModelTag, session, info, newPolicy, clock, runTransactionObserver)
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

// mongodbLogin logs in to the mongodb admin database.
func mongodbLogin(session *mgo.Session, mongoInfo *mongo.MongoInfo) error {
	admin := session.DB("admin")
	if mongoInfo.Tag != nil {
		if err := admin.Login(mongoInfo.Tag.String(), mongoInfo.Password); err != nil {
			return maybeUnauthorized(err, fmt.Sprintf("cannot log in to admin database as %q", mongoInfo.Tag))
		}
	} else if mongoInfo.Password != "" {
		if err := admin.Login(mongo.AdminUser, mongoInfo.Password); err != nil {
			return maybeUnauthorized(err, "cannot log in to admin database")
		}
	}
	return nil
}

func maybeUnauthorized(err error, msg string) error {
	if err == nil {
		return nil
	}
	if isUnauthorized(err) {
		return errors.Unauthorizedf("%s: unauthorized mongo access: %v", msg, err)
	}
	return errors.Annotatef(err, msg)
}

func isUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	// Some unauthorized access errors have no error code,
	// just a simple error string; and some do have error codes
	// but are not of consistent types (LastError/QueryError).
	for _, prefix := range []string{"auth fail", "not authorized", "server returned error on SASL authentication step: Authentication failed."} {
		if strings.HasPrefix(err.Error(), prefix) {
			return true
		}
	}
	if err, ok := err.(*mgo.QueryError); ok {
		return err.Code == 10057 ||
			err.Code == 13 ||
			err.Message == "need to login" ||
			err.Message == "unauthorized"
	}
	return false
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
	session *mgo.Session, mongoInfo *mongo.MongoInfo,
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
	}

	// Create State.
	st := &State{
		stateClock:             clock,
		modelTag:               modelTag,
		controllerModelTag:     controllerModelTag,
		mongoInfo:              mongoInfo,
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

// MongoConnectionInfo returns information for connecting to mongo
func (st *State) MongoConnectionInfo() *mongo.MongoInfo {
	return st.mongoInfo
}

// CACert returns the certificate used to validate the state connection.
func (st *State) CACert() string {
	return st.mongoInfo.CACert
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
