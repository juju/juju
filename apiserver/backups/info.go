// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Info provides the implementation of the API method.
func (a *API) Info(args params.BackupsInfoArgs) (params.BackupsMetadataResult, error) {
	backups, closer := newBackups(a.st)
	defer closer.Close()

	meta, file, err := backups.Get(args.ID)
	if err != nil {
		return params.BackupsMetadataResult{}, errors.Trace(err)
	}
	if file != nil {
		// We don't use the archive file but need to close it
		// nonetheless or else we'll leak sockets.
		defer file.Close()
	}

	return ResultFromMetadata(meta), nil
}
