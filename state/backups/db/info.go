// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db

import (
	"github.com/juju/errors"

	"github.com/juju/juju/mongo"
)

// ConnInfo is a simplification of authentication.MongoInfo.
type connInfo struct {
	address  string
	username string
	password string
}

// NewConnInfo returns a new DB connection info value.
func NewConnInfo(addr, user, pw string) *connInfo {
	info := connInfo{
		address:  addr,
		username: user,
		password: pw,
	}
	return &info
}

// NewMongoConnInfo returns a new DB connection info value based on the
// mongo info.
func NewMongoConnInfo(mgoInfo *mongo.MongoInfo) *connInfo {
	info := connInfo{
		address:  mgoInfo.Addrs[0],
		password: mgoInfo.Password,
	}

	// TODO(dfc) Backup should take a Tag.
	if mgoInfo.Tag != nil {
		info.username = mgoInfo.Tag.String()
	}

	return &info
}

// Check returns the DB connection info, ensuring it is valid.
func (ci *connInfo) Check() (address, username, password string, err error) {
	address = ci.address
	username = ci.username
	password = ci.password

	if address == "" {
		err = errors.New("missing address")
	} else if username == "" {
		err = errors.New("missing username")
	} else if password == "" {
		err = errors.New("missing password")
	}

	return address, username, password, err
}
