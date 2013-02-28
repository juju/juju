package state

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"time"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/state/watcher"
)

// Info encapsulates information about cluster of
// servers holding juju state and can be used to make a
// connection to that cluster.
type Info struct {
	// Addrs gives the addresses of the MongoDB servers for the state.
	// Each address should be in the form address:port.
	Addrs []string

	// CACert holds the CA certificate that will be used
	// to validate the state server's certificate, in PEM format.
	CACert []byte

	// EntityName holds the name of the entity that is connecting.
	// It should be empty when connecting as an administrator.
	EntityName string

	// Password holds the password for the connecting entity.
	Password string
}

var dialTimeout = 10 * time.Minute

// Open connects to the server described by the given
// info, waits for it to be initialized, and returns a new State
// representing the environment connected to.
// It returns ErrUnauthorized if access is unauthorized.
func Open(info *Info) (*State, error) {
	log.Printf("state: opening state; mongo addresses: %q; entity %q", info.Addrs, info.EntityName)
	if len(info.Addrs) == 0 {
		return nil, errors.New("no mongo addresses")
	}
	if len(info.CACert) == 0 {
		return nil, errors.New("missing CA certificate")
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
		log.Printf("state: connecting to %v", addr)
		c, err := tls.Dial("tcp", addr.String(), tlsConfig)
		if err != nil {
			log.Printf("state: connection failed: %v", err)
			return nil, err
		}
		log.Printf("state: connection established")
		return c, err
	}
	session, err := mgo.DialWithInfo(&mgo.DialInfo{
		Addrs:   info.Addrs,
		Timeout: dialTimeout,
		Dial:    dial,
	})
	if err != nil {
		return nil, err
	}
	st, err := newState(session, info)
	if err != nil {
		session.Close()
		return nil, err
	}
	return st, nil
}

// Initialize sets up an initial empty state and returns it.
// This needs to be performed only once for a given environment.
// It returns ErrUnauthorized if access is unauthorized.
func Initialize(info *Info, cfg *config.Config) (rst *State, err error) {
	st, err := Open(info)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			st.Close()
		}
	}()
	// A valid environment config is used as a signal that the
	// state has already been initalized. If this is the case
	// do nothing.
	if _, err := st.EnvironConfig(); err == nil {
		return st, nil
	} else if !IsNotFound(err) {
		return nil, err
	}
	log.Printf("state: initializing environment")
	if cfg.AdminSecret() != "" {
		return nil, fmt.Errorf("admin-secret should never be written to the state")
	}
	ops := []txn.Op{
		createConstraintsOp(st, "e", Constraints{}),
		createSettingsOp(st, "e", cfg.AllAttrs()),
	}
	if err := st.runner.Run(ops, "", nil); err == txn.ErrAborted {
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
}{
	// After the first public release, do not remove entries from here
	// without adding them to a list of indexes to drop, to ensure
	// old databases are modified to have the correct indexes.
	{"relations", []string{"endpoints.relationname"}},
	{"relations", []string{"endpoints.servicename"}},
	{"units", []string{"service"}},
	{"units", []string{"principal"}},
	{"units", []string{"machineid"}},
	{"users", []string{"name"}},
}

// The capped collection used for transaction logs defaults to 10MB.
// It's tweaked in export_test.go to 1MB to avoid the overhead of
// creating and deleting the large file repeatedly in tests.
var (
	logSize      = 10000000
	logSizeTests = 1000000
)

var ErrUnauthorized = errors.New("unauthorized access")

func maybeUnauthorized(err error, msg string) error {
	if err == nil {
		return nil
	}
	// Unauthorized access errors have no error code,
	// just a simple error string.
	if err.Error() == "auth fails" {
		return ErrUnauthorized
	}
	if err, ok := err.(*mgo.QueryError); ok && err.Code == 10057 {
		return ErrUnauthorized
	}
	return fmt.Errorf("%s: %v", msg, err)
}

func newState(session *mgo.Session, info *Info) (*State, error) {
	db := session.DB("juju")
	pdb := session.DB("presence")
	if info.EntityName != "" {
		if err := db.Login(info.EntityName, info.Password); err != nil {
			return nil, maybeUnauthorized(err, "cannot log in to juju database")
		}
		if err := pdb.Login(info.EntityName, info.Password); err != nil {
			return nil, maybeUnauthorized(err, "cannot log in to presence database")
		}
	} else if info.Password != "" {
		admin := session.DB("admin")
		if err := admin.Login("admin", info.Password); err != nil {
			return nil, maybeUnauthorized(err, "cannot log in to admin database")
		}
	}
	st := &State{
		info:           info,
		db:             db,
		charms:         db.C("charms"),
		machines:       db.C("machines"),
		relations:      db.C("relations"),
		relationScopes: db.C("relationscopes"),
		services:       db.C("services"),
		settings:       db.C("settings"),
		constraints:    db.C("constraints"),
		units:          db.C("units"),
		users:          db.C("users"),
		presence:       pdb.C("presence"),
		cleanups:       db.C("cleanups"),
	}
	log := db.C("txns.log")
	logInfo := mgo.CollectionInfo{Capped: true, MaxBytes: logSize}
	// The lack of error code for this error was reported upstream:
	//     https://jira.mongodb.org/browse/SERVER-6992
	err := log.Create(&logInfo)
	if err != nil && err.Error() != "collection already exists" {
		return nil, maybeUnauthorized(err, "cannot create log collection")
	}
	st.runner = txn.NewRunner(db.C("txns"))
	st.runner.ChangeLog(db.C("txns.log"))
	st.watcher = watcher.New(db.C("txns.log"))
	st.pwatcher = presence.NewWatcher(pdb.C("presence"))
	for _, item := range indexes {
		index := mgo.Index{Key: item.key}
		if err := db.C(item.collection).EnsureIndex(index); err != nil {
			return nil, fmt.Errorf("cannot create database index: %v", err)
		}
	}
	return st, nil
}

// Addrs returns the list of addresses used to connect to the state.
func (st *State) Addrs() (addrs []string) {
	return append(addrs, st.info.Addrs...)
}

// CACert returns the certificate used to validate the state connection.
func (st *State) CACert() (cert []byte) {
	return append(cert, st.info.CACert...)
}

func (st *State) Close() error {
	err1 := st.watcher.Stop()
	err2 := st.pwatcher.Stop()
	st.db.Session.Close()
	for _, err := range []error{err1, err2} {
		if err != nil {
			return err
		}
	}
	return nil
}
