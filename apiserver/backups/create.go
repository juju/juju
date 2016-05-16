// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"
	"github.com/juju/replicaset"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
)

var waitUntilReady = replicaset.WaitUntilReady

type Machine interface {
	Series() string
}

func machine(st *state.State, ID string) (Machine, error) {
	m, err := st.Machine(ID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return m, nil
}

var getMachine = machine

// Create is the API method that requests juju to create a new backup
// of its state.  It returns the metadata for that backup.
func (a *API) Create(args params.BackupsCreateArgs) (p params.BackupsMetadataResult, err error) {
	backupsMethods, closer := newBackups(a.st)
	defer closer.Close()

	session := a.st.MongoSession().Copy()
	defer session.Close()

	// Don't go if HA isn't ready.
	err = waitUntilReady(session, 60)
	if err != nil {
		return p, errors.Annotatef(err, "HA not ready; try again later")
	}

	mgoInfo := a.st.MongoConnectionInfo()
	dbInfo, err := backups.NewDBInfo(mgoInfo, session)
	if err != nil {
		return p, errors.Trace(err)
	}
	m, err := getMachine(a.st, a.machineID)
	if err != nil {
		return p, errors.Trace(err)
	}

	meta, err := backups.NewMetadataState(a.st, a.machineID, m.Series())
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
