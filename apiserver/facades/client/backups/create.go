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
//
// NOTE(hml) this exists only for backwards compatibility,
// for API facade versions 1; clients should prefer its
// successor, CreateBackup, below. Until all consumers
// have been updated, or we bump a major version, we can't
// drop this.
//
// TODO(hml) 2017-05-03
// Drop this in Juju 3.0.
func (a *APIv2) Create(args params.BackupsCreateArgs) (p params.BackupsCreateResult, _ error) {
	return a.CreateBackup(args)
}

// Create is the API method that requests juju to create a new backup
// of its state.  It returns the metadata for that backup.
//
// NOTE(hml) this provides backwards compatibility for facade version 1.
func (a *API) Create(args params.BackupsCreateArgs) (p params.BackupsMetadataResult, err error) {
	args.KeepCopy = true
	args.NoDownload = true
	result, err := a.APIv2.Create(args)
	if err != nil {
		return p, errors.Trace(err)
	}
	return result.Metadata, nil
}

func (a *APIv2) CreateBackup(args params.BackupsCreateArgs) (p params.BackupsCreateResult, _ error) {
	backupsMethods, closer := newBackups(a.backend)
	defer closer.Close()

	session := a.backend.MongoSession().Copy()
	defer session.Close()

	// Don't go if HA isn't ready.
	err := waitUntilReady(session, 60)
	if err != nil {
		return p, errors.Annotatef(err, "HA not ready; try again later")
	}

	mgoInfo, err := mongoInfo(a.paths.DataDir, a.machineID)
	if err != nil {
		return p, errors.Annotatef(err, "getting mongo info")
	}
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

	fileName, err := backupsMethods.Create(meta, a.paths, dbInfo, args.KeepCopy, args.NoDownload)
	if err != nil {
		return p, errors.Trace(err)
	}

	return params.BackupsCreateResult{
		Metadata: ResultFromMetadata(meta),
		Filename: fileName,
	}, nil
}
