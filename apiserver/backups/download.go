// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Download provides the implementation of the API method.
func (b *API) DownloadDirect(args params.BackupsDownloadArgs) (params.BackupsDownloadDirectResult, error) {
	var result params.BackupsDownloadDirectResult

	_, archive, err := b.backups.Get(args.ID)
	if err != nil {
		return result, errors.Trace(err)
	}

	if archive == nil {
		return result, errors.Errorf("backup for %q missing archive", args.ID)
	}

	var data bytes.Buffer
	_, err = data.ReadFrom(archive)
	if err != nil {
		return result, errors.Trace(err)
	}
	result.Data = data.Bytes()

	return result, nil
}
