package state

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
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

	// UseSSH specifies whether MongoDB should be contacted through an
	// SSH tunnel.
	UseSSH bool

	// EntityName holds the name of the entity that is connecting.
	// It should be empty when connecting as an administrator.
	EntityName string

	// Password holds the password for the administrator or connecting entity.
	Password string
}

// Open connects to the server described by the given
// info, waits for it to be initialized, and returns a new State
// representing the environment connected to.
// It returns ErrUnauthorized if access is unauthorized.
func Open(info *Info) (*State, error) {
	log.Printf("state: opening state; mongo addresses: %q", info.Addrs)
	if len(info.Addrs) == 0 {
		return nil, errors.New("no mongo addresses")
	}
	var (
		session *mgo.Session
		fwd     *sshForwarder
		err     error
	)
	if info.UseSSH {
		// TODO implement authorization on SSL connection; drop sshDial.
		if len(info.Addrs) > 1 {
			return nil, errors.New("ssh connect does not support multiple addresses")
		}
		fwd, session, err = sshDial(info.Addrs[0], "")
	} else {
		session, err = mgo.DialWithTimeout(strings.Join(info.Addrs, ","), 10*time.Minute)
	}
	if err != nil {
		return nil, err
	}
	st, err := newState(session, fwd, info.EntityName, info.Password)
	if err != nil {
		session.Close()
		return nil, err
	}
	return st, nil
}

// Initialize sets up an initial empty state and returns it.
// This needs to be performed only once for a given environment.
// It returns ErrUnauthorized if access is unauthorized.
func Initialize(info *Info, cfg *config.Config) (*State, error) {
	st, err := Open(info)
	if err != nil {
		return nil, err
	}
	// A valid environment config is used as a signal that the
	// state has already been initalized. If this is the case
	// do nothing.
	if _, err = st.EnvironConfig(); !IsNotFound(err) {
		return st, nil
	}
	log.Printf("state: storing no-secrets environment configuration")
	if _, err = createSettings(st, "e", nil); err != nil {
		st.Close()
		return nil, err
	}
	if err = st.SetEnvironConfig(cfg); err != nil {
		st.Close()
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

func newState(session *mgo.Session, fwd *sshForwarder, entity, password string) (*State, error) {
	db := session.DB("juju")
	pdb := session.DB("presence")
	if entity != "" {
		if err := db.Login(entity, password); err != nil {
			return nil, maybeUnauthorized(err, "cannot log in to juju database")
		}
		if err := pdb.Login(entity, password); err != nil {
			return nil, maybeUnauthorized(err, "cannot log in to presence database")
		}
	} else if password != "" {
		admin := session.DB("admin")
		if err := admin.Login("admin", password); err != nil {
			return nil, maybeUnauthorized(err, "cannot log in to admin database")
		}
	}
	st := &State{
		db:             db,
		charms:         db.C("charms"),
		machines:       db.C("machines"),
		relations:      db.C("relations"),
		relationScopes: db.C("relationscopes"),
		services:       db.C("services"),
		settings:       db.C("settings"),
		units:          db.C("units"),
		presence:       pdb.C("presence"),
		cleanups:       db.C("cleanups"),
		fwd:            fwd,
	}
	log := db.C("txns.log")
	info := mgo.CollectionInfo{Capped: true, MaxBytes: logSize}
	// The lack of error code for this error was reported upstream:
	//     https://jira.mongodb.org/browse/SERVER-6992
	err := log.Create(&info)
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
