package mstate

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/mstate/presence"
	"launchpad.net/juju-core/mstate/watcher"
)

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
	db := session.DB("juju")
	pdb := session.DB("presence")
	st := &State{
		db:         db,
		charms:     db.C("charms"),
		machines:   db.C("machines"),
		relations:  db.C("relations"),
		services:   db.C("services"),
		settings:   db.C("settings"),
		units:      db.C("units"),
		presence:   pdb.C("presence"),
	}
	st.runner = txn.NewRunner(db.C("txns"))
	st.runner.ChangeLog(db.C("txns.log"))
	st.watcher = watcher.New(db.C("txns.log"))
	st.presencew = presence.NewWatcher(pdb.C("presence"))
	for _, index := range indexes {
		err = st.relations.EnsureIndex(index)
		if err != nil {
			return nil, err
		}
	}
	return st, nil
}

func (st *State) Close() error {
	err1 := st.presencew.Stop()
	err2 := st.watcher.Stop()
	st.db.Session.Close()
	for _, err := range []error{err1, err2} {
		if err != nil {
			return err
		}
	}
	return nil
}
