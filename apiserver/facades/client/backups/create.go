// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/replicaset/v3"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/backups"
)

var waitUntilReady = func(s *mgo.Session, timeout int) error {
	return replicaset.WaitUntilReady(s, timeout)
}

// Create is the API method that requests juju to create a new backup
// of its state.
func (a *API) Create(args params.BackupsCreateArgs) (params.BackupsMetadataResult, error) {
	backupsMethods := newBackups(a.paths)

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
	dbInfo, err := backups.NewDBInfo(mgoInfo, sessionShim{session})
	if err != nil {
		return result, errors.Trace(err)
	}
	mBase, err := a.backend.MachineBase(a.machineID)
	if err != nil {
		return result, errors.Trace(err)
	}

	meta, err := backups.NewMetadataState(a.backend, a.machineID, mBase.DisplayString())
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

	fileName, err := backupsMethods.Create(meta, dbInfo)
	if err != nil {
		return result, errors.Trace(err)
	}

	result = params.CreateResult(meta, fileName)
	return result, nil
}
