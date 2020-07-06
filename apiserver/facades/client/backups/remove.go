// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	commonerrors "github.com/juju/juju/apiserver/common/errors"
	"github.com/juju/juju/apiserver/params"
)

// Remove deletes the backups defined by ID from the database.
func (a *APIv2) Remove(args params.BackupsRemoveArgs) (params.ErrorResults, error) {
	backups, closer := newBackups(a.backend)
	defer closer.Close()
	results := make([]params.ErrorResult, len(args.IDs))
	for i, id := range args.IDs {
		err := backups.Remove(id)
		results[i].Error = commonerrors.ServerError(err)
	}
	return params.ErrorResults{results}, nil
}
