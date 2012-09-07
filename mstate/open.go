package mstate

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/mstate/presence"
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
	presencedb := session.DB("presence")
	txns := db.C("txns")
	st := &State{
		db:         db,
		presencedb: presencedb,
		charms:     db.C("charms"),
		machines:   db.C("machines"),
		relations:  db.C("relations"),
		services:   db.C("services"),
		settings:   db.C("settings"),
		units:      db.C("units"),
		presence:   presencedb.C("presence"),
		runner:     txn.NewRunner(txns),
	}
	st.presencew = presence.NewWatcher(st.presence)
	for _, index := range indexes {
		err = st.relations.EnsureIndex(index)
		if err != nil {
			return nil, err
		}
	}
	return st, nil
}

func (st *State) Close() error {
	err := st.presencew.Stop()
	st.db.Session.Close()
	return err
}
