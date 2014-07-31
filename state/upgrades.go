// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/loggo"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var upgradesLogger = loggo.GetLogger("juju.state.upgrade")

type userDocBefore struct {
	Name           string    `bson:"_id"`
	LastConnection time.Time `bson:"lastconnection"`
}

func MigrateUserLastConnectionToLastLogin(st *State) error {
	var oldDocs []userDocBefore

	err := st.ResumeTransactions()
	if err != nil {
		return err
	}

	users, closer := st.getCollection(usersC)
	defer closer()
	err = users.Find(bson.D{{
		"lastconnection", bson.D{{"$exists", true}}}}).All(&oldDocs)
	if err != nil {
		return err
	}

	var zeroTime time.Time

	ops := []txn.Op{}
	for _, oldDoc := range oldDocs {
		upgradesLogger.Debugf("updating user %q", oldDoc.Name)
		var lastLogin *time.Time
		if oldDoc.LastConnection != zeroTime {
			lastLogin = &oldDoc.LastConnection
		}
		ops = append(ops,
			txn.Op{
				C:      usersC,
				Id:     oldDoc.Name,
				Assert: txn.DocExists,
				Update: bson.D{
					{"$set", bson.D{{"lastlogin", lastLogin}}},
					{"$unset", bson.D{{"lastconnection", nil}}},
					{"$unset", bson.D{{"_id_", nil}}},
				},
			})
	}

	return st.runTransaction(ops)
}
