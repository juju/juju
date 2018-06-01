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

	backups, closer := newBackups(a.backend)
	defer closer.Close()

	metaList, err := backups.List()
	if err != nil {
		return result, errors.Trace(err)
	}

	result.List = make([]params.BackupsMetadataResult, len(metaList))
	for i, meta := range metaList {
		result.List[i] = CreateResult(meta, "")
	}

	return result, nil
}
