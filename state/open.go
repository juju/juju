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
// representing the environment connected to.
//
// A policy may be provided, which will be used to validate and
// modify behaviour of certain operations in state. A nil policy
// may be provided.
//
// Open returns unauthorizedError if access is unauthorized.
func Open(tag names.EnvironTag, info *mongo.MongoInfo, opts mongo.DialOpts, policy Policy) (*State, error) {
	st, err := open(tag, info, opts, policy)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, err := st.Environment(); err != nil {
		if err := st.Close(); err != nil {
			logger.Errorf("error closing state for unreadable environment %s: %v", tag.Id(), err)
		}
		return nil, errors.Annotatef(err, "cannot read environment %s", tag.Id())
	}

	// State should only be Opened on behalf of a state server environ; all
	// other *States should be created via ForEnviron.
	if err := st.start(tag); err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

func open(tag names.EnvironTag, info *mongo.MongoInfo, opts mongo.DialOpts, policy Policy) (*State, error) {
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
	// environment UUID available. In that case we need to infer what it might be;
	// we depend on the assumption that this is the only circumstance in which
	// the the UUID might not be known.
	if tag.Id() == "" {
		logger.Warningf("creating state without environment tag; inferring bootstrap environment")
		ssInfo, err := readRawStateServerInfo(session)
		if err != nil {
			session.Close()
			return nil, errors.Trace(err)
		}
		tag = ssInfo.EnvironmentTag
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
// This needs to be performed only once for the initial state server environment.
// It returns unauthorizedError if access is unauthorized.
func Initialize(owner names.UserTag, info *mongo.MongoInfo, cfg *config.Config, opts mongo.DialOpts, policy Policy) (_ *State, err error) {
	uuid, ok := cfg.UUID()
	if !ok {
		return nil, errors.Errorf("environment uuid was not supplied")
	}
	envTag := names.NewEnvironTag(uuid)
	st, err := open(envTag, info, opts, policy)
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

	// A valid environment is used as a signal that the
	// state has already been initalized. If this is the case
	// do nothing.
	if _, err := st.Environment(); err == nil {
		return nil, errors.New("already initialized")
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	// When creating the state server environment, the new environment
	// UUID is also used as the state server UUID.
	logger.Infof("initializing state server environment %s", uuid)
	ops, err := st.envSetupOps(cfg, uuid, uuid, owner)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops,
		createInitialUserOp(st, owner, info.Password),
		txn.Op{
			C:      stateServersC,
			Id:     environGlobalKey,
			Assert: txn.DocMissing,
			Insert: &stateServersDoc{
				EnvUUID: st.EnvironUUID(),
			},
		},
		txn.Op{
			C:      stateServersC,
			Id:     apiHostPortsKey,
			Assert: txn.DocMissing,
			Insert: &apiHostPortsDoc{},
		},
		txn.Op{
			C:      stateServersC,
			Id:     stateServingInfoKey,
			Assert: txn.DocMissing,
			Insert: &StateServingInfo{},
		},
		txn.Op{
			C:      stateServersC,
			Id:     hostedEnvCountKey,
			Assert: txn.DocMissing,
			Insert: &hostedEnvCountDoc{},
		},
	)

	if err := st.runTransaction(ops); err != nil {
		return nil, errors.Trace(err)
	}
	if err := st.start(envTag); err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

func (st *State) envSetupOps(cfg *config.Config, envUUID, serverUUID string, owner names.UserTag) ([]txn.Op, error) {
	if err := checkEnvironConfig(cfg); err != nil {
		return nil, errors.Trace(err)
	}

	// When creating the state server environment, the new environment
	// UUID is also used as the state server UUID.
	if serverUUID == "" {
		serverUUID = envUUID
	}
	envUserOp := createEnvUserOp(envUUID, owner, owner, owner.Name())
	ops := []txn.Op{
		createConstraintsOp(st, environGlobalKey, constraints.Value{}),
		createSettingsOp(st, environGlobalKey, cfg.AllAttrs()),
		incHostedEnvironCountOp(),
		createEnvironmentOp(st, owner, cfg.Name(), envUUID, serverUUID),
		createUniqueOwnerEnvNameOp(owner, cfg.Name()),
		envUserOp,
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
// pwatcher, leadershipManager, or serverTag. You must start() the returned
// *State before it will function correctly.
func newState(environTag names.EnvironTag, session *mgo.Session, mongoInfo *mongo.MongoInfo, policy Policy) (_ *State, resultErr error) {
	// Set up database.
	rawDB := session.DB(jujuDB)
	database, err := allCollections().Load(rawDB, environTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := InitDbLogs(session); err != nil {
		return nil, errors.Trace(err)
	}

	// Create State.
	return &State{
		environTag: environTag,
		mongoInfo:  mongoInfo,
		session:    session,
		database:   database,
		policy:     policy,
		watcher:    watcher.New(rawDB.C(txnLogC)),
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
	st.mu.Lock()
	if st.allManager != nil {
		handle("allwatcher manager", st.allManager.Stop())
	}
	if st.allEnvManager != nil {
		handle("allenvwatcher manager", st.allEnvManager.Stop())
	}
	if st.allEnvWatcherBacking != nil {
		handle("allenvwatcher backing", st.allEnvWatcherBacking.Release())
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
