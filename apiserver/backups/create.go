// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/backups"
)

// Create is the API method that requests juju to create a new backup
// of its state.  It returns the metadata for that backup.
func (a *API) Create(args params.BackupsCreateArgs) (p params.BackupsMetadataResult, err error) {
	backupsMethods, closer := newBackups(a.st)
	defer closer.Close()

	dbInfo, err := func() (*backups.DBInfo, error) {
		mgoInfo := a.st.MongoConnectionInfo()
		session := a.st.MongoSession().Copy()
		defer session.Close()
		dbInfo, err := backups.NewDBInfo(mgoInfo, session)
		return dbInfo, errors.Trace(err)
	}()
	if err != nil {
		return p, err
	}

	meta, err := backups.NewMetadataState(a.st, a.machineID)
	if err != nil {
		return p, errors.Trace(err)
	}
	meta.Notes = args.Notes

	err = backupsMethods.Create(meta, a.paths, dbInfo)
	if err != nil {
		return p, errors.Trace(err)
	}

	p.UpdateFromMetadata(meta)

	return p, nil
}
