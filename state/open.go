// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/watcher"
)

// Open connects to the server described by the given
// info, waits for it to be initialized, and returns a new State
// representing the model connected to.
//
// A policy may be provided, which will be used to validate and
// modify behaviour of certain operations in state. A nil policy
// may be provided.
//
// Open returns unauthorizedError if access is unauthorized.
func Open(tag names.ModelTag, info *mongo.MongoInfo, opts mongo.DialOpts, policy Policy) (*State, error) {
	st, err := open(tag, info, opts, policy)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, err := st.Model(); err != nil {
		if err := st.Close(); err != nil {
			logger.Errorf("error closing state for unreadable model %s: %v", tag.Id(), err)
		}
		return nil, errors.Annotatef(err, "cannot read model %s", tag.Id())
	}

	// State should only be Opened on behalf of a controller environ; all
	// other *States should be created via ForEnviron.
	if err := st.start(tag); err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

func open(tag names.ModelTag, info *mongo.MongoInfo, opts mongo.DialOpts, policy Policy) (*State, error) {
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

	// In rare circumstances, we may be upgrading from pre-1.23, and not have the
	// model UUID available. In that case we need to infer what it might be;
	// we depend on the assumption that this is the only circumstance in which
	// the the UUID might not be known.
	if tag.Id() == "" {
		logger.Warningf("creating state without model tag; inferring bootstrap model")
		ssInfo, err := readRawControllerInfo(session)
		if err != nil {
			session.Close()
			return nil, errors.Trace(err)
		}
		tag = ssInfo.ModelTag
	}

	st, err := newState(tag, session, info, policy)
	if err != nil {
		session.Close()
		return nil, errors.Trace(err)
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

// Initialize sets up an initial empty state and returns it.
// This needs to be performed only once for the initial controller model.
// It returns unauthorizedError if access is unauthorized.
func Initialize(owner names.UserTag, info *mongo.MongoInfo, cfg *config.Config, opts mongo.DialOpts, policy Policy) (_ *State, err error) {
	uuid, ok := cfg.UUID()
	if !ok {
		return nil, errors.Errorf("model uuid was not supplied")
	}
	modelTag := names.NewModelTag(uuid)
	st, err := open(modelTag, info, opts, policy)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			if closeErr := st.Close(); closeErr != nil {
				logger.Errorf("error closing state while aborting Initialize: %v", closeErr)
			}
		}
	}()

	// A valid model is used as a signal that the
	// state has already been initalized. If this is the case
	// do nothing.
	if _, err := st.Model(); err == nil {
		return nil, errors.New("already initialized")
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	// When creating the controller model, the new model
	// UUID is also used as the controller UUID.
	logger.Infof("initializing controller model %s", uuid)
	ops, err := st.envSetupOps(cfg, uuid, uuid, owner)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops,
		createInitialUserOp(st, owner, info.Password),
		txn.Op{
			C:      controllersC,
			Id:     modelGlobalKey,
			Assert: txn.DocMissing,
			Insert: &controllersDoc{
				ModelUUID: st.ModelUUID(),
			},
		},
		txn.Op{
			C:      controllersC,
			Id:     apiHostPortsKey,
			Assert: txn.DocMissing,
			Insert: &apiHostPortsDoc{},
		},
		txn.Op{
			C:      controllersC,
			Id:     stateServingInfoKey,
			Assert: txn.DocMissing,
			Insert: &StateServingInfo{},
		},
		txn.Op{
			C:      controllersC,
			Id:     hostedModelCountKey,
			Assert: txn.DocMissing,
			Insert: &hostedModelCountDoc{},
		},
	)

	if err := st.runTransaction(ops); err != nil {
		return nil, errors.Trace(err)
	}
	if err := st.start(modelTag); err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

func (st *State) envSetupOps(cfg *config.Config, modelUUID, serverUUID string, owner names.UserTag) ([]txn.Op, error) {
	if err := checkModelConfig(cfg); err != nil {
		return nil, errors.Trace(err)
	}

	// When creating the controller model, the new model
	// UUID is also used as the controller UUID.
	if serverUUID == "" {
		serverUUID = modelUUID
	}
	modelUserOp := createModelUserOp(modelUUID, owner, owner, owner.Name(), nowToTheSecond(), false)
	ops := []txn.Op{
		createConstraintsOp(st, modelGlobalKey, constraints.Value{}),
		createSettingsOp(modelGlobalKey, cfg.AllAttrs()),
		incHostedModelCountOp(),
		createModelOp(st, owner, cfg.Name(), modelUUID, serverUUID),
		createUniqueOwnerModelNameOp(owner, cfg.Name()),
		modelUserOp,
	}
	return ops, nil
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
	for _, prefix := range []string{"auth fail", "not authorized"} {
		if strings.HasPrefix(err.Error(), prefix) {
			return true
		}
	}
	if err, ok := err.(*mgo.QueryError); ok {
		return err.Code == 10057 ||
			err.Message == "need to login" ||
			err.Message == "unauthorized"
	}
	return false
}

// newState creates an incomplete *State, with a configured watcher but no
// pwatcher, leadershipManager, or controllerTag. You must start() the returned
// *State before it will function correctly.
func newState(modelTag names.ModelTag, session *mgo.Session, mongoInfo *mongo.MongoInfo, policy Policy) (_ *State, resultErr error) {
	// Set up database.
	rawDB := session.DB(jujuDB)
	database, err := allCollections().Load(rawDB, modelTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := InitDbLogs(session); err != nil {
		return nil, errors.Trace(err)
	}

	// Create State.
	return &State{
		modelTag:  modelTag,
		mongoInfo: mongoInfo,
		session:   session,
		database:  database,
		policy:    policy,
		watcher:   watcher.New(rawDB.C(txnLogC)),
	}, nil
}

// MongoConnectionInfo returns information for connecting to mongo
func (st *State) MongoConnectionInfo() *mongo.MongoInfo {
	return st.mongoInfo
}

// CACert returns the certificate used to validate the state connection.
func (st *State) CACert() string {
	return st.mongoInfo.CACert
}

func (st *State) Close() (err error) {
	defer errors.DeferredAnnotatef(&err, "closing state failed")

	// TODO(fwereade): we have no defence against these components failing
	// and leaving other parts of state going. They should be managed by a
	// dependency.Engine (or perhaps worker.Runner).
	var errs []error
	handle := func(name string, err error) {
		if err != nil {
			errs = append(errs, errors.Annotatef(err, "error stopping %s", name))
		}
	}

	handle("transaction watcher", st.watcher.Stop())
	if st.pwatcher != nil {
		handle("presence watcher", st.pwatcher.Stop())
	}
	if st.leadershipManager != nil {
		st.leadershipManager.Kill()
		handle("leadership manager", st.leadershipManager.Wait())
	}
	if st.singularManager != nil {
		st.singularManager.Kill()
		handle("singular manager", st.singularManager.Wait())
	}
	st.mu.Lock()
	if st.allManager != nil {
		handle("allwatcher manager", st.allManager.Stop())
	}
	if st.allModelManager != nil {
		handle("allModelWatcher manager", st.allModelManager.Stop())
	}
	if st.allModelWatcherBacking != nil {
		handle("allModelWatcher backing", st.allModelWatcherBacking.Release())
	}
	st.session.Close()
	st.mu.Unlock()

	if len(errs) > 0 {
		for _, err := range errs[1:] {
			logger.Errorf("while closing state: %v", err)
		}
		return errs[0]
	}
	logger.Debugf("closed state without error")
	return nil
}
