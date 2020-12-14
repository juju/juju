// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/replicaset"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/backups"
)

var waitUntilReady = replicaset.WaitUntilReady

func (a *API) Create(args params.BackupsCreateArgs) (params.BackupsMetadataResult, error) {
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
	dbInfo, err := backups.NewDBInfo(mgoInfo, session)
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
	meta.Controller.MachineID = a.machineID
	m, err := a.backend.Machine(a.machineID)
	if err != nil {
		return result, errors.Trace(err)
	}
	instanceID, err := m.InstanceId()
	if err != nil {
		return result, errors.Trace(err)
	}
	meta.Controller.MachineInstanceID = string(instanceID)

	nodes, err := a.backend.ControllerNodes()
	if err != nil {
		return result, errors.Trace(err)
	}
	meta.Controller.HANodes = int64(len(nodes))

	fileName, err := backupsMethods.Create(meta, a.paths, dbInfo, args.KeepCopy, args.NoDownload)
	if err != nil {
		return result, errors.Trace(err)
	}

	result = CreateResult(meta, fileName)
	return result, nil
}
