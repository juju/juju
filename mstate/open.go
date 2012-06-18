package mstate

import (
	"launchpad.net/juju-core/juju/log"
	"launchpad.net/mgo"
)

func Dial(servers string) (st *State, err error) {
	log.Printf("state: opening state; MongoDB servers: %q", servers)
	session, err := mgo.Dial(servers)
	if err != nil {
		return
	}
	db := session.DB("juju")
	db.Session = session.Clone()
	session.Close()
	st = &State{
		db:       db,
		machines: db.C("machines"),
	}
	return
}

func (st *State) Close() (err error) {
	st.db.Session.Close()
	return
}
