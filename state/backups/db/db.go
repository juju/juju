// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// db is a surrogate for the proverbial DB layer abstraction that we
// wish we had for juju state.  To that end, the package holds the DB
// implementation-specific details and functionality needed for backups.
// Currently that means mongo-specific details.  However, as a stand-in
// for a future DB layer abstraction, the db package does not expose any
// low-level details publicly.  Thus the backups implementation remains
// oblivious to the underlying DB implementation.
package db

import (
	"github.com/juju/errors"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	coreutils "github.com/juju/juju/utils"
)

var runCommand = coreutils.RunCommand

// NewDBBackupInfo returns the information needed by backups to dump
// the database.
func NewDBBackupInfo(st *state.State) (*Info, error) {
	connInfo := newMongoConnInfo(st.MongoConnectionInfo())
	targets, err := state.GetBackupTargetDatabases(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	info := Info{
		ConnInfo: *connInfo,
		Targets:  targets,
	}
	return &info, nil
}

func newMongoConnInfo(mgoInfo *mongo.MongoInfo) *ConnInfo {
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
