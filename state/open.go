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
	st.startPresenceWatcher()
	return st, nil
}

func open(tag names.EnvironTag, info *mongo.MongoInfo, opts mongo.DialOpts, policy Policy) (*State, error) {
	logger.Infof("opening state, mongo addresses: %q; entity %v", info.Addrs, info.Tag)
	logger.Debugf("dialing mongo")
	session, err := mongo.DialWithInfo(info.Info, opts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("connection established")

	st, err := newState(tag, session, info, policy)
	if err != nil {
		session.Close()
		return nil, errors.Trace(err)
	}
	return st, nil
}

// Initialize sets up an initial empty state and returns it.
// This needs to be performed only once for the initial state server environment.
// It returns unauthorizedError if access is unauthorized.
func Initialize(owner names.UserTag, info *mongo.MongoInfo, cfg *config.Config, opts mongo.DialOpts, policy Policy) (rst *State, err error) {
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
			st.Close()
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
	logger.Infof("starting presence watcher")
	st.startPresenceWatcher()

	// When creating the state server environment, the new environment
	// UUID is also used as the state server UUID.
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
	)

	if err := st.runTransaction(ops); err != nil {
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
	envUserOp, _ := createEnvUserOpAndDoc(envUUID, owner, owner, owner.Name())
	ops := []txn.Op{
		createConstraintsOp(st, environGlobalKey, constraints.Value{}),
		createSettingsOp(st, environGlobalKey, cfg.AllAttrs()),
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
	return errors.Annotatef(err, "%s: %v", msg, err)
}

func isUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	// Some unauthorized access errors have no error code,
	// just a simple error string.
	if strings.HasPrefix(err.Error(), "auth fail") {
		return true
	}
	if err, ok := err.(*mgo.QueryError); ok {
		return err.Code == 10057 ||
			err.Message == "need to login" ||
			err.Message == "unauthorized" ||
			strings.HasPrefix(err.Message, "not authorized")
	}
	return false
}

func newState(environTag names.EnvironTag, session *mgo.Session, mongoInfo *mongo.MongoInfo, policy Policy) (_ *State, resultErr error) {
	admin := session.DB("admin")
	if mongoInfo.Tag != nil {
		if err := admin.Login(mongoInfo.Tag.String(), mongoInfo.Password); err != nil {
			return nil, maybeUnauthorized(err, fmt.Sprintf("cannot log in to admin database as %q", mongoInfo.Tag))
		}
	} else if mongoInfo.Password != "" {
		if err := admin.Login(mongo.AdminUser, mongoInfo.Password); err != nil {
			return nil, maybeUnauthorized(err, "cannot log in to admin database")
		}
	}

	// Set up database.
	rawDB := session.DB(jujuDB)
	database, err := allCollections.Load(rawDB, environTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := InitDbLogs(session); err != nil {
		return nil, errors.Trace(err)
	}

	// Create State.
	st := &State{
		environTag: environTag,
		serverTag:  environTag,
		mongoInfo:  mongoInfo,
		session:    session,
		database:   database,
		policy:     policy,
		watcher:    watcher.New(rawDB.C(txnLogC)),
	}
	st.LeasePersistor = NewLeasePersistor(leaseC, st.run, st.getCollection)
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

func (st *State) Close() (err error) {
	defer errors.DeferredAnnotatef(&err, "closing state failed")
	err1 := st.watcher.Stop()
	var err2 error
	if st.pwatcher != nil {
		err2 = st.pwatcher.Stop()
	}
	st.mu.Lock()
	var err3 error
	if st.allManager != nil {
		err3 = st.allManager.Stop()
	}
	st.session.Close()
	st.mu.Unlock()
	var i int
	for i, err = range []error{err1, err2, err3} {
		if err != nil {
			switch i {
			case 0:
				err = errors.Annotatef(err, "failed to stop state watcher")
			case 1:
				err = errors.Annotatef(err, "failed to stop presence watcher")
			case 2:
				err = errors.Annotatef(err, "failed to stop all manager")
			}
			return err
		}
	}
	return nil
}
