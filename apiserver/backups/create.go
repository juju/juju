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

	mgoInfo := a.st.MongoConnectionInfo()
	dbInfo, err := backups.NewDBInfo(mgoInfo, a.st.MongoSession())
	if err != nil {
		return p, errors.Trace(err)
	}

	// TODO(ericsnow) lp-1389362
	// The machine ID needs to be introspected from the API server, likely
	// through a Resource.
	const machineID = "0"

	meta, err := backups.NewMetadataState(a.st, machineID)
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
