// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
)

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

	err = st.users.Find(bson.D{{
		"lastconnection", bson.D{{"$exists", true}}}}).All(&oldDocs)
	if err != nil {
		return err
	}

	var zeroTime time.Time

	ops := []txn.Op{}
	for _, oldDoc := range oldDocs {
		var lastLogin *time.Time
		if oldDoc.LastConnection != zeroTime {
			lastLogin = &oldDoc.LastConnection
		}

		ops = append(ops,
			txn.Op{
				C:      userCollectionName,
				Id:     oldDoc.Name,
				Assert: txn.DocExists,
				Update: bson.D{
					{"$set", bson.D{{"lastlogin", lastLogin}}},
					{"$unset", bson.D{{"lastconnection", nil}}},
				},
			})
	}

	return st.runTransaction(ops)
}
