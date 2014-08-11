// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environmentserver/authentication"
)

// DBConnInfo is a simplification of authentication.MongoInfo.
type DBConnInfo interface {
	// Address returns the connection address.
	Address() string
	// Username returns the connection username.
	Username() string
	// Password returns the connection password.
	Password() string
	// UpdateFromMongoInfo pulls in the provided connection info.
	UpdateFromMongoInfo(mgoInfo *authentication.MongoInfo)
	// Check returns the info after ensuring it's okay.
	Check() (address, username, password string, err error)
}

type dbConnInfo struct {
	address  string
	username string
	password string
}

// NewDBConnInfo returns a new DBConnInfo.
func NewDBConnInfo(addr, user, pw string) DBConnInfo {
	dbinfo := dbConnInfo{
		address:  addr,
		username: user,
		password: pw,
	}
	return &dbinfo
}

func (ci *dbConnInfo) Address() string {
	return ci.address
}

func (ci *dbConnInfo) Username() string {
	return ci.username
}

func (ci *dbConnInfo) Password() string {
	return ci.password
}

func (ci *dbConnInfo) UpdateFromMongoInfo(mgoInfo *authentication.MongoInfo) {
	ci.address = mgoInfo.Addrs[0]
	ci.password = mgoInfo.Password

	// TODO(dfc) Backup should take a Tag.
	if mgoInfo.Tag != nil {
		ci.username = mgoInfo.Tag.String()
	}
}

func (ci *dbConnInfo) Check() (address, username, password string, err error) {
	address = ci.address
	username = ci.username
	password = ci.password

	if address == "" {
		err = errors.Errorf("missing address")
	} else if username == "" {
		err = errors.Errorf("missing username")
	} else if password == "" {
		err = errors.Errorf("missing password")
	}

	return
}
