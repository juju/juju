// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/rpc/params"
)

// Create is the API method that requests juju to create a new backup
// of its state.
func (a *API) Create(context.Context, params.BackupsCreateArgs) (params.BackupsMetadataResult, error) {
	result := params.BackupsMetadataResult{}
	return result, errors.NotImplementedf("Dqlite-based backups")
}
