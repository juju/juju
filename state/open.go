// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"crypto/tls"
	"crypto/x509"
	stderrors "errors"
	"fmt"
	"net"
	"time"

	"github.com/juju/errors"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/utils"
)

// mongoSocketTimeout should be long enough that
// even a slow mongo server will respond in that
// length of time. Since mongo servers ping themselves
// every 10 seconds, that seems like a reasonable
// default.
const mongoSocketTimeout = 10 * time.Second

// defaultDialTimeout should be representative of
// the upper bound of time taken to dial a mongo
// server from within the same cloud/private network.
const defaultDialTimeout = 30 * time.Second

// Info encapsulates information about cluster of
// servers holding juju state and can be used to make a
// connection to that cluster.
type Info struct {
	// Addrs gives the addresses of the MongoDB servers for the state.
	// Each address should be in the form address:port.
	Addrs []string

	// CACert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	CACert string

	// Tag holds the name of the entity that is connecting.
	// It should be empty when connecting as an administrator.
	Tag string

	// Password holds the password for the connecting entity.
	Password string
}

// DialOpts holds configuration parameters that control the
// Dialing behavior when connecting to a state server.
type DialOpts struct {
	// Timeout is the amount of time to wait contacting
	// a state server.
	Timeout time.Duration
}

// DefaultDialOpts returns a DialOpts representing the default
// parameters for contacting a state server.
func DefaultDialOpts() DialOpts {
	return DialOpts{Timeout: defaultDialTimeout}
}

// Open connects to the server described by the given
// info, waits for it to be initialized, and returns a new State
// representing the environment connected to.
//
// A policy may be provided, which will be used to validate and
// modify behaviour of certain operations in state. A nil policy
// may be provided.
//
// Open returns unauthorizedError if access is unauthorized.
func Open(info *Info, opts DialOpts, policy Policy) (*State, error) {
	logger.Infof("opening state, mongo addresses: %q; entity %q", info.Addrs, info.Tag)
	di, err := DialInfo(info, opts)
	if err != nil {
		return nil, err
	}
	logger.Debugf("dialing mongo")
	session, err := mgo.DialWithInfo(di)
	if err != nil {
		return nil, err
	}
	logger.Debugf("connection established")

	st, err := newState(session, info, policy)
	if err != nil {
		session.Close()
		return nil, err
	}
	session.SetSocketTimeout(mongoSocketTimeout)
	return st, nil
}

// DialInfo returns information on how to dial
// the state's mongo server with the given info
// and dial options.
func DialInfo(info *Info, opts DialOpts) (*mgo.DialInfo, error) {
	if len(info.Addrs) == 0 {
		return nil, stderrors.New("no mongo addresses")
	}
	if len(info.CACert) == 0 {
		return nil, stderrors.New("missing CA certificate")
	}
	xcert, err := cert.ParseCert(info.CACert)
	if err != nil {
		return nil, fmt.Errorf("cannot parse CA certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(xcert)
	tlsConfig := &tls.Config{
		RootCAs:    pool,
		ServerName: "anything",
	}
	dial := func(addr net.Addr) (net.Conn, error) {
		c, err := net.Dial("tcp", addr.String())
		if err != nil {
			logger.Debugf("connection failed, will retry: %v", err)
			return nil, err
		}
		cc := tls.Client(c, tlsConfig)
		if err := cc.Handshake(); err != nil {
			logger.Errorf("TLS handshake failed: %v", err)
			return nil, err
		}
		logger.Infof("dialled mongo successfully")
		return cc, nil
	}

	return &mgo.DialInfo{
		Addrs:   info.Addrs,
		Timeout: opts.Timeout,
		Dial:    dial,
	}, nil
}

// Initialize sets up an initial empty state and returns it.
// This needs to be performed only once for a given environment.
// It returns unauthorizedError if access is unauthorized.
func Initialize(info *Info, cfg *config.Config, opts DialOpts, policy Policy) (rst *State, err error) {
	st, err := Open(info, opts, policy)
	if err != nil {
		return nil, err
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
		return st, nil
	} else if !errors.IsNotFound(err) {
		return nil, err
	}
	logger.Infof("initializing environment")
	if err := checkEnvironConfig(cfg); err != nil {
		return nil, err
	}
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, fmt.Errorf("environment UUID cannot be created: %v", err)
	}
	ops := []txn.Op{
		createConstraintsOp(st, environGlobalKey, constraints.Value{}),
		createSettingsOp(st, environGlobalKey, cfg.AllAttrs()),
		createEnvironmentOp(st, cfg.Name(), uuid.String()),
		{
			C:      st.stateServers.Name,
			Id:     environGlobalKey,
			Insert: &stateServersDoc{},
		}, {
			C:      st.stateServers.Name,
			Id:     apiHostPortsKey,
			Insert: &apiHostPortsDoc{},
		},
	}
	if err := st.runTransaction(ops); err == txn.ErrAborted {
		// The config was created in the meantime.
		return st, nil
	} else if err != nil {
		return nil, err
	}
	return st, nil
}

var indexes = []struct {
	collection string
	key        []string
	unique     bool
}{
	// After the first public release, do not remove entries from here
	// without adding them to a list of indexes to drop, to ensure
	// old databases are modified to have the correct indexes.
	{"relations", []string{"endpoints.relationname"}, false},
	{"relations", []string{"endpoints.servicename"}, false},
	{"units", []string{"service"}, false},
	{"units", []string{"principal"}, false},
	{"units", []string{"machineid"}, false},
	{"users", []string{"name"}, false},
	{"networks", []string{"providerid"}, true},
	{"networkinterfaces", []string{"interfacename", "machineid"}, true},
	{"networkinterfaces", []string{"macaddress", "networkname"}, true},
	{"networkinterfaces", []string{"networkname"}, false},
	{"networkinterfaces", []string{"machineid"}, false},
}

// The capped collection used for transaction logs defaults to 10MB.
// It's tweaked in export_test.go to 1MB to avoid the overhead of
// creating and deleting the large file repeatedly in tests.
var (
	logSize      = 10000000
	logSizeTests = 1000000
)

func maybeUnauthorized(err error, msg string) error {
	if err == nil {
		return nil
	}
	if isUnauthorized(err) {
		return errors.Unauthorizedf("%s: unauthorized mongo access: %v", msg, err)
	}
	return fmt.Errorf("%s: %v", msg, err)
}

func isUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	// Some unauthorized access errors have no error code,
	// just a simple error string.
	if err.Error() == "auth fails" {
		return true
	}
	if err, ok := err.(*mgo.QueryError); ok {
		return err.Code == 10057 ||
			err.Message == "need to login" ||
			err.Message == "unauthorized"
	}
	return false
}

func newState(session *mgo.Session, info *Info, policy Policy) (*State, error) {
	db := session.DB("juju")
	pdb := session.DB("presence")
	admin := session.DB("admin")
	if info.Tag != "" {
		if err := db.Login(info.Tag, info.Password); err != nil {
			return nil, maybeUnauthorized(err, fmt.Sprintf("cannot log in to juju database as %q", info.Tag))
		}
		if err := pdb.Login(info.Tag, info.Password); err != nil {
			return nil, maybeUnauthorized(err, fmt.Sprintf("cannot log in to presence database as %q", info.Tag))
		}
		if err := admin.Login(info.Tag, info.Password); err != nil {
			return nil, maybeUnauthorized(err, fmt.Sprintf("cannot log in to admin database as %q", info.Tag))
		}
	} else if info.Password != "" {
		if err := admin.Login(AdminUser, info.Password); err != nil {
			return nil, maybeUnauthorized(err, "cannot log in to admin database")
		}
	}

	st := &State{
		info:              info,
		policy:            policy,
		db:                db,
		environments:      db.C("environments"),
		charms:            db.C("charms"),
		machines:          db.C("machines"),
		containerRefs:     db.C("containerRefs"),
		instanceData:      db.C("instanceData"),
		relations:         db.C("relations"),
		relationScopes:    db.C("relationscopes"),
		services:          db.C("services"),
		requestedNetworks: db.C("requestednetworks"),
		networks:          db.C("networks"),
		networkInterfaces: db.C("networkinterfaces"),
		minUnits:          db.C("minunits"),
		settings:          db.C("settings"),
		settingsrefs:      db.C("settingsrefs"),
		constraints:       db.C("constraints"),
		units:             db.C("units"),
		actions:           db.C("actions"),
		users:             db.C("users"),
		presence:          pdb.C("presence"),
		cleanups:          db.C("cleanups"),
		annotations:       db.C("annotations"),
		statuses:          db.C("statuses"),
		stateServers:      db.C("stateServers"),
	}
	log := db.C("txns.log")
	logInfo := mgo.CollectionInfo{Capped: true, MaxBytes: logSize}
	// The lack of error code for this error was reported upstream:
	//     https://jira.klmongodb.org/browse/SERVER-6992
	err := log.Create(&logInfo)
	if err != nil && err.Error() != "collection already exists" {
		return nil, maybeUnauthorized(err, "cannot create log collection")
	}
	st.runner = txn.NewRunner(db.C("txns"))
	st.runner.ChangeLog(db.C("txns.log"))
	st.watcher = watcher.New(db.C("txns.log"))
	st.pwatcher = presence.NewWatcher(pdb.C("presence"))
	for _, item := range indexes {
		index := mgo.Index{Key: item.key, Unique: item.unique}
		if err := db.C(item.collection).EnsureIndex(index); err != nil {
			return nil, fmt.Errorf("cannot create database index: %v", err)
		}
	}
	st.transactionHooks = make(chan ([]transactionHook), 1)
	st.transactionHooks <- nil

	// TODO(rog) delete this when we can assume there are no
	// pre-1.18 environments running.
	if err := st.createStateServersDoc(); err != nil {
		return nil, fmt.Errorf("cannot create state servers document: %v", err)
	}
	if err := st.createAPIAddressesDoc(); err != nil {
		return nil, fmt.Errorf("cannot create API addresses document: %v", err)
	}
	if err := st.createStateServingInfoDoc(); err != nil {
		return nil, fmt.Errorf("cannot create state serving info document: %v", err)
	}
	return st, nil
}

// createStateServersDoc creates the state servers document
// if it does not already exist. This is necessary to cope with
// legacy environments that have not created the document
// at initialization time.
func (st *State) createStateServersDoc() error {
	// Quick check to see if we need to do anything so
	// that we can avoid transaction overhead in most cases.
	// We don't care what the error is - if it's something
	// unexpected, it'll be picked up again below.
	if info, err := st.StateServerInfo(); err == nil {
		if len(info.MachineIds) > 0 && len(info.VotingMachineIds) > 0 {
			return nil
		}
	}
	logger.Infof("adding state server info to legacy environment")
	// Find all current state servers and add the state servers
	// record containing them. We don't need to worry about
	// this being concurrent-safe, because in the juju versions
	// we're concerned about, there is only ever one state connection
	// (from the single bootstrap machine).
	var machineDocs []machineDoc
	err := st.machines.Find(bson.D{{"jobs", JobManageEnviron}}).All(&machineDocs)
	if err != nil {
		return err
	}
	var doc stateServersDoc
	for _, m := range machineDocs {
		doc.MachineIds = append(doc.MachineIds, m.Id)
	}
	doc.VotingMachineIds = doc.MachineIds
	logger.Infof("found existing state servers %v", doc.MachineIds)

	// We update the document before inserting it because
	// an earlier version of this code did not insert voting machine
	// ids or maintain the ids correctly. If that was the case,
	// the insert will be a no-op.
	ops := []txn.Op{{
		C:  st.stateServers.Name,
		Id: environGlobalKey,
		Update: bson.D{{"$set", bson.D{
			{"machineids", doc.MachineIds},
			{"votingmachineids", doc.VotingMachineIds},
		}}},
	}, {
		C:      st.stateServers.Name,
		Id:     environGlobalKey,
		Insert: &doc,
	}}

	return st.runTransaction(ops)
}

// createAPIAddressesDoc creates the API addresses document
// if it does not already exist. This is necessary to cope with
// legacy environments that have not created the document
// at initialization time.
func (st *State) createAPIAddressesDoc() error {
	var doc apiHostPortsDoc
	ops := []txn.Op{{
		C:      st.stateServers.Name,
		Id:     apiHostPortsKey,
		Assert: txn.DocMissing,
		Insert: &doc,
	}}
	return onAbort(st.runTransaction(ops), nil)
}

// createStateServingInfoDoc creates the state serving info document
// if it does not already exist
func (st *State) createStateServingInfoDoc() error {
	var info params.StateServingInfo
	ops := []txn.Op{{
		C:      st.stateServers.Name,
		Id:     stateServingInfoKey,
		Assert: txn.DocMissing,
		Insert: &info,
	}}
	return onAbort(st.runTransaction(ops), nil)
}

// CACert returns the certificate used to validate the state connection.
func (st *State) CACert() string {
	return st.info.CACert
}

func (st *State) Close() error {
	err1 := st.watcher.Stop()
	err2 := st.pwatcher.Stop()
	st.mu.Lock()
	var err3 error
	if st.allManager != nil {
		err3 = st.allManager.Stop()
	}
	st.mu.Unlock()
	st.db.Session.Close()
	for _, err := range []error{err1, err2, err3} {
		if err != nil {
			return err
		}
	}
	return nil
}
