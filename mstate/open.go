package mstate

import (
	"launchpad.net/juju-core/juju/log"
	"launchpad.net/mgo"
)

func Dial(servers string) (st *State, err error) {
	log.Printf("opening state with servers: %q", servers)
	session, err := mgo.Dial(servers)
	if err != nil {
		return
	}
	db := session.DB("juju")
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
