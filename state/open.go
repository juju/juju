// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v6"

	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
)

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
	db := &database{}
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
	st.controllerTag = controllerTag
	return st, nil
}

// Close the connection to the database.
func (st *State) Close() (err error) {
	return nil
}
