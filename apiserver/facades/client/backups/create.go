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
// NOTE(hml) this provides backwards compatibility for facade version 1.
func (a *API) Create(args params.BackupsCreateArgs) (params.BackupsMetadataResult, error) {
	args.KeepCopy = true
	args.NoDownload = true

	apiv2 := APIv2{a}
	result, err := apiv2.Create(args)
	if err != nil {
		return result, errors.Trace(err)
	}
	return result, nil
}

func (a *APIv2) Create(args params.BackupsCreateArgs) (params.BackupsMetadataResult, error) {
	backupsMethods, closer := newBackups(a.backend)
	defer closer.Close()

	session := a.backend.MongoSession().Copy()
	defer session.Close()

	result := params.BackupsMetadataResult{}
	// Don't go if HA isn't ready.
	err := waitUntilReady(session, 60)
	if err != nil {
		return result, errors.Annotatef(err, "HA not ready; try again later")
	}

	mgoInfo, err := mongoInfo(a.paths.DataDir, a.machineID)
	if err != nil {
		return result, errors.Annotatef(err, "getting mongo info")
	}
	v, err := a.backend.MongoVersion()
	if err != nil {
		return result, errors.Annotatef(err, "discovering mongo version")
	}
	mongoVersion, err := mongo.NewVersion(v)
	if err != nil {
		return result, errors.Trace(err)
	}
	dbInfo, err := backups.NewDBInfo(mgoInfo, session, mongoVersion)
	if err != nil {
		return result, errors.Trace(err)
	}
	mSeries, err := a.backend.MachineSeries(a.machineID)
	if err != nil {
		return result, errors.Trace(err)
	}

	meta, err := backups.NewMetadataState(a.backend, a.machineID, mSeries)
	if err != nil {
		return result, errors.Trace(err)
	}
	meta.Notes = args.Notes

	fileName, err := backupsMethods.Create(meta, a.paths, dbInfo, args.KeepCopy, args.NoDownload)
	if err != nil {
		return result, errors.Trace(err)
	}

	result = CreateResult(meta, fileName)
	return result, nil
}
