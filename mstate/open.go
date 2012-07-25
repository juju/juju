package mstate

import (
	"labix.org/v2/mgo"
	"launchpad.net/juju-core/log"
)

func Dial(servers string) (*State, error) {
	log.Printf("opening state with servers: %q", servers)
	session, err := mgo.Dial(servers)
	if err != nil {
		return nil, err
	}
	db := session.DB("juju")
	st := &State{
		db:        db,
		charms:    db.C("charms"),
		machines:  db.C("machines"),
		relations: db.C("relations"),
		services:  db.C("services"),
		units:     db.C("units"),
	}
	err = st.relations.EnsureIndexKey("endpoints.relationname")
	if err != nil {
		return nil, err
	}
	err = st.relations.EnsureIndexKey("endpoints.servicename")
	if err != nil {
		return nil, err
	}
	return st, nil
}

func (st *State) Close() (err error) {
	st.db.Session.Close()
	return
}
