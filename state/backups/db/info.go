// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db

import (
	"github.com/juju/errors"

	"github.com/juju/juju/mongo"
)

// ConnInfo is a simplification of authentication.MongoInfo, focused
// on the needs of juju state backups.
type ConnInfo struct {
	// Address is the DB system's host address.
	Address string
	// Username is used when connecting to the DB system.
	Username string
	// Password is used when connecting to the DB system.
	Password string
}

// NewMongoConnInfo returns a new DB connection info value based on the
// mongo info.
func NewMongoConnInfo(mgoInfo *mongo.MongoInfo) *ConnInfo {
	info := ConnInfo{
		Address:  mgoInfo.Addrs[0],
		Password: mgoInfo.Password,
	}

	// TODO(dfc) Backup should take a Tag.
	if mgoInfo.Tag != nil {
		info.Username = mgoInfo.Tag.String()
	}

	return &info
}

// Check returns the DB connection info, ensuring it is valid.
func (ci *ConnInfo) Check() (address, username, password string, err error) {
	address = ci.Address
	username = ci.Username
	password = ci.Password

	if address == "" {
		err = errors.New("missing address")
	} else if username == "" {
		err = errors.New("missing username")
	} else if password == "" {
		err = errors.New("missing password")
	}

	return address, username, password, err
}
