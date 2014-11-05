// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// Create is the API method that requests juju to create a new backup
// of its state.  It returns the metadata for that backup.
func (a *API) Create(args params.BackupsCreateArgs) (p params.BackupsMetadataResult, err error) {
	backups, closer := newBackups(a.st)
	defer closer.Close()

	dbInfo, err := state.NewDBBackupInfo(a.st)
	if err != nil {
		return p, errors.Trace(err)
	}

	// TODO(ericsnow) #1389362 The machine ID needs to be introspected
	// from the API server, likely through a Resource.
	machine := "0"
	origin, err := state.NewBackupOrigin(a.st, machine)
	if err != nil {
		return p, errors.Trace(err)
	}

	meta, err := backups.Create(a.paths, *dbInfo, *origin, args.Notes)
	if err != nil {
		return p, errors.Trace(err)
	}

	p.UpdateFromMetadata(meta)

	return p, nil
}
