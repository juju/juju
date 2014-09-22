// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/state/backupstorage"
)

// Create is the API method that requests juju to create a new backup
// of its state.  It returns the metadata for that backup.
func (a *API) Create(args params.BackupsCreateArgs) (p params.BackupsMetadataResult, err error) {
	envStor, err := environs.GetStorage(a.st)
	if err != nil {
		return p, errors.Trace(err)
	}
	stor := newBackupsStorage(a.st, envStor)
	defer stor.Close()
	backups := newBackups(stor)

	mgoInfo := a.st.MongoConnectionInfo()
	dbInfo := db.NewMongoConnInfo(mgoInfo)

	machine := "0" // We *could* pull this from state.
	origin := backupstorage.NewBackupsOrigin(a.st, machine)

	meta, err := backups.Create(*dbInfo, *origin, args.Notes)
	if err != nil {
		return p, errors.Trace(err)
	}

	p.UpdateFromMetadata(meta)

	return p, nil
}
