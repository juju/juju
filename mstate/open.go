package mstate

import (
	"labix.org/v2/mgo"
	"launchpad.net/juju-core/log"
)

var indexes = []mgo.Index{
	{Key: []string{"key"}, Unique: true},
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
	st := &State{
		db:        db,
		charms:    db.C("charms"),
		machines:  db.C("machines"),
		relations: db.C("relations"),
		services:  db.C("services"),
		units:     db.C("units"),
	}
	for _, index := range indexes {
		err = st.relations.EnsureIndex(index)
		if err != nil {
			return nil, err
		}
	}
	return st, nil
}

func (st *State) Close() (err error) {
	st.db.Session.Close()
	return
}
