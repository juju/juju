// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// List provides the implementation of the API method.
func (a *API) List(args params.BackupsListArgs) (params.BackupsListResult, error) {
	var result params.BackupsListResult

	backups, closer := newBackups(a.st)
	defer closer.Close()

	metaList, err := backups.List()
	if err != nil {
		return result, errors.Trace(err)
	}

	result.List = make([]params.BackupsMetadataResult, len(metaList))
	for i, meta := range metaList {
		var resultItem params.BackupsMetadataResult
		resultItem.UpdateFromMetadata(meta)
		result.List[i] = resultItem
	}

	return result, nil
}
