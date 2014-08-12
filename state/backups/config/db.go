// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/mongo"
)

// TODO(ericsnow) Pull these from elsewhere in juju?
var defaultDBDumpName = "mongodump"

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

// DBInfo is an abstraction of the DB-related info needed for backups.
type DBInfo interface {
	// ConnInfo returns the DB connection info needed for backups.
	ConnInfo() DBConnInfo
	// BinDir returns the directory containing the DB executable files.
	BinDir() string
	// DumpBinary returns the path to the DB dump executable file.
	DumpBinary() string
}

type dbInfo struct {
	connInfo DBConnInfo
	binDir   string
	dumpName string
}

func newDBInfo(connInfo DBConnInfo) (*dbInfo, error) {
	info := dbInfo{
		connInfo: connInfo,
		binDir:   "",
		dumpName: defaultDBDumpName,
	}

	mongod, err := mongo.Path()
	if err != nil {
		return &info, errors.Annotate(err, "failed to get mongod path")
	}
	info.binDir = filepath.Dir(mongod)

	return &info, nil
}

// NewDBInfo returns a new DBInfo value with default values.
func NewDBInfo(connInfo DBConnInfo) (DBInfo, error) {
	info, err := newDBInfo(connInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return info, nil
}

// NewDBInfoFull returns a new DBInfo value.
func NewDBInfoFull(addr, user, pw, binDir, dumpName string) (DBInfo, error) {
	connInfo := NewDBConnInfo(addr, user, pw)
	info, err := newDBInfo(connInfo)
	if err != nil {
		if info == nil || binDir == "" {
			return nil, errors.Trace(err)
		}
	}
	if binDir == "" {
		if info.binDir == "" {
			return nil, errors.Errorf("missing binDir")
		}
		binDir = info.binDir
	}
	if dumpName == "" {
		dumpName = info.dumpName
	}

	info.binDir = binDir
	info.dumpName = dumpName

	return info, nil
}

func (db *dbInfo) ConnInfo() DBConnInfo {
	return db.connInfo
}

func (db *dbInfo) BinDir() string {
	return db.binDir
}

func (db *dbInfo) DumpBinary() string {
	if db.dumpName == "" {
		return ""
	}
	if db.binDir == "" {
		return db.dumpName
	}
	return filepath.Join(db.binDir, db.dumpName)
}
