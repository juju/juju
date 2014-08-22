// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

func (b *API) Info(args params.BackupsInfoArgs) (params.BackupsMetadataResult, error) {
	var result params.BackupsMetadataResult

	meta, _, err := b.backups.Get(args.ID)
	if err != nil {
		return result, errors.Trace(err)
	}

	result.UpdateFromMetadata(meta)

	return result, nil
}
