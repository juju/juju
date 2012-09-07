package mstate

import (
	"errors"
	"strings"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/log"
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
}

// Open connects to the server described by the given
// info, waits for it to be initialized, and returns a new State
// representing the environment connected to.
func Open(info *Info) (*State, error) {
	st, err := open(info)
	if err != nil {
		return nil, err
	}
	log.Printf("mstate: waiting for state to be initialized")
	// TODO(dfc) wait for the /environment key 
	return st, err
}

func open(info *Info) (*State, error) {
	log.Printf("mstate: opening state; mongo addresses: %q", info.Addrs)
	if len(info.Addrs) == 0 {
		return nil, errors.New("no mongo addresses")
	}
	if !info.UseSSH {
		return Dial(strings.Join(info.Addrs, ","))
	}
	if len(info.Addrs) > 1 {
		return nil, errors.New("ssh connect does not support multiple addresses")
	}
	fwd, session, err := sshDial(info.Addrs[0], "")
	if err != nil {
		return nil, err
	}
	return newState(session, fwd)
}

var indexes = []mgo.Index{
	{Key: []string{"endpoints.relationname"}},
	{Key: []string{"endpoints.servicename"}},
}

func Dial(servers string) (*State, error) {
	log.Printf("opening state with servers: %q", servers)
	session, err := mgo.Dial(servers)
	if err != nil {
		return nil, err
	}
	return newState(session, nil)
}

func newState(session *mgo.Session, fwd *sshForwarder) (*State, error) {
	db := session.DB("juju")
	txns := db.C("txns")
	st := &State{
		db:        db,
		charms:    db.C("charms"),
		machines:  db.C("machines"),
		relations: db.C("relations"),
		services:  db.C("services"),
		settings:  db.C("settings"),
		units:     db.C("units"),
		runner:    txn.NewRunner(txns),
		fwd:       fwd,
	}
	for _, index := range indexes {
		if err := st.relations.EnsureIndex(index); err != nil {
			return nil, err
		}
	}
	return st, nil
}

func (st *State) Close() error {
	st.db.Session.Close()
	if st.fwd != nil {
		if err := st.fwd.stop(); err != nil {
			return err
		}
	}
	return nil
}
