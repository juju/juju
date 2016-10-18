// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/replicaset"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/backups"
)

var waitUntilReady = replicaset.WaitUntilReady

// Create is the API method that requests juju to create a new backup
// of its state.  It returns the metadata for that backup.
func (a *API) Create(args params.BackupsCreateArgs) (p params.BackupsMetadataResult, err error) {
	backupsMethods, closer := newBackups(a.backend)
	defer closer.Close()

	session := a.backend.MongoSession().Copy()
	defer session.Close()

	// Don't go if HA isn't ready.
	err = waitUntilReady(session, 60)
	if err != nil {
		return p, errors.Annotatef(err, "HA not ready; try again later")
	}

	mgoInfo := a.backend.MongoConnectionInfo()
	v, err := a.backend.MongoVersion()
	if err != nil {
		return p, errors.Annotatef(err, "discovering mongo version")
	}
	mongoVersion, err := mongo.NewVersion(v)
	if err != nil {
		return p, errors.Trace(err)
	}
	dbInfo, err := backups.NewDBInfo(mgoInfo, session, mongoVersion)
	if err != nil {
		return p, errors.Trace(err)
	}
	mSeries, err := a.backend.MachineSeries(a.machineID)
	if err != nil {
		return p, errors.Trace(err)
	}

	meta, err := backups.NewMetadataState(a.backend, a.machineID, mSeries)
	if err != nil {
		return p, errors.Trace(err)
	}
	meta.Notes = args.Notes

	err = backupsMethods.Create(meta, a.paths, dbInfo)
	if err != nil {
		return p, errors.Trace(err)
	}

	return ResultFromMetadata(meta), nil
}
