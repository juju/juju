// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package db

import (
	"github.com/juju/errors"

	"github.com/juju/juju/mongo"
)

// ConnInfo is a simplification of authentication.MongoInfo, focused
// on the needs of juju state backups.  To ensure that the info is valid
// for use in backups, use the Check() method to get the contained
// values.
type ConnInfo struct {
	// Address is the DB system's host address.
	Address string
	// Username is used when connecting to the DB system.
	Username string
	// Password is used when connecting to the DB system.
	Password string
}

// Validate checks the DB connection info.  If it isn't valid for use in
// juju state backups, it returns an error.  Make sure that the ConnInfo
// values do not change between the time you call this method and when
// you actually need the values.
func (ci *ConnInfo) Validate() error {
	var err error
	var address, username, password string

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

	return err
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
