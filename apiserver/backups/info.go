// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Info provides the implementation of the API method.
func (a *API) Info(args params.BackupsInfoArgs) (params.BackupsMetadataResult, error) {
	var result params.BackupsMetadataResult

	backups, closer := newBackups(a.st)
	defer closer.Close()

	meta, _, err := backups.Get(args.ID) // Ignore the archive file.
	if err != nil {
		return result, errors.Trace(err)
	}

	result.UpdateFromMetadata(meta)

	return result, nil
}
