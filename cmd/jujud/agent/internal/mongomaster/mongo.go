// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongomaster

import (
	mgo "gopkg.in/mgo.v2"

	"github.com/juju/juju/mongo"
)

// MongoConn is an implementation of Conn, reporting whether or not the
// replicaset member identified by Member is the master.
type MongoConn struct {
	Session *mgo.Session
	Member  mongo.WithAddresses
}

// Ping is part of the master.Conn interface.
func (c *MongoConn) Ping() error {
	return c.Session.Ping()
}

// IsMember is part of the master.Conn interface.
func (c *MongoConn) IsMaster() (bool, error) {
	return mongo.IsMaster(c.Session, c.Member)
}
