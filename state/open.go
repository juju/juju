// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
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

	// InitDatabaseFunc, if non-nil, is a function that will be called
	// just after the state database is opened.
	InitDatabaseFunc InitDatabaseFunc

	// MaxTxnAttempts is defaulted by OpenStatePool if otherwise not set.
	MaxTxnAttempts int

	// Note(nvinuesa): Having a dqlite domain service here is an awful hack
	// and should disapear as soon as we migrate units and applications.
	CharmServiceGetter func(modelUUID coremodel.UUID) (CharmService, error)

	// WatcherPollInterval is defaulted by the TxnWatcher if otherwise not set.
	WatcherPollInterval time.Duration
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
	controllerTag names.ControllerTag,
	controllerModelTag names.ModelTag,
	session *mgo.Session,
	initDatabase InitDatabaseFunc,
	controllerConfig *controller.Config,
	newPolicy NewPolicyFunc,
	clock clock.Clock,
	charmServiceGetter func(modelUUID coremodel.UUID) (CharmService, error),
	maxTxnAttempts int,
) (*State, error) {
	st, err := newState(controllerTag,
		controllerModelTag,
		controllerModelTag,
		session,
		newPolicy,
		clock,
		charmServiceGetter,
		maxTxnAttempts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if initDatabase != nil {
		if err := initDatabase(session, controllerModelTag.Id(), controllerConfig); err != nil {
			session.Close()
			return nil, errors.Trace(err)
		}
		logger.Debugf(context.TODO(), "mongodb initialised")
	}

	return st, nil
}

// newState creates an incomplete *State, with no running workers.
// You must startWorkers() the returned *State before it will function correctly.
// modelTag is used to filter all queries and transactions.
//
// newState takes responsibility for the supplied *mgo.Session, and will
// close it if it cannot be returned under the aegis of a *State.
func newState(
	controllerTag names.ControllerTag,
	modelTag, controllerModelTag names.ModelTag,
	session *mgo.Session,
	newPolicy NewPolicyFunc,
	clock clock.Clock,
	charmServiceGetter func(modelUUID coremodel.UUID) (CharmService, error),
	maxTxnAttempts int,
) (_ *State, err error) {

	defer func() {
		if err != nil {
			session.Close()
		}
	}()

	info, err := session.BuildInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(info.VersionArray) < 1 || info.VersionArray[0] < 4 {
		parts := make([]string, len(info.VersionArray))
		for i, v := range info.VersionArray {
			parts[i] = fmt.Sprintf("%d", v)
		}
		mongoVers := "this mongo version"
		if len(parts) > 0 {
			mongoVers = fmt.Sprintf("mongo version %v", strings.Join(parts, "."))
		}
		return nil, errors.Errorf("%v does not support server side transactions", mongoVers)
	}
	mongodb := session.DB(jujuDB)
	db := &database{
		raw:            mongodb,
		schema:         allCollections(),
		modelUUID:      modelTag.Id(),
		clock:          clock,
		maxTxnAttempts: maxTxnAttempts,
	}

	// Create State.
	st := &State{
		stateClock:         clock,
		modelTag:           modelTag,
		controllerModelTag: controllerModelTag,
		session:            session,
		database:           db,
		newPolicy:          newPolicy,
		charmServiceGetter: charmServiceGetter,
		maxTxnAttempts:     maxTxnAttempts,
	}
	if newPolicy != nil {
		st.policy = newPolicy(st)
	}
	// Record this State instance with the global tracker.
	profileTracker.Add(st, 1)

	st.controllerTag = controllerTag
	return st, nil
}

// Close the connection to the database.
func (st *State) Close() (err error) {
	defer errors.DeferredAnnotatef(&err, "closing state failed")
	if err := st.stopWorkers(); err != nil {
		return errors.Trace(err)
	}
	st.session.Close()
	logger.Debugf(context.TODO(), "closed state without error")
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
