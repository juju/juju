// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"io/ioutil"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Download provides the implementation of the API method.
func (b *API) DownloadDirect(args params.BackupsDownloadArgs) (params.BackupsDownloadResult, error) {
	var result params.BackupsDownloadResult

	_, archive, err := b.backups.Get(args.ID)
	if err != nil {
		return result, errors.Trace(err)
	}

	if archive == nil {
		return result, errors.Errorf("backup for %q missing archive", args.ID)
	}

	var copied bytes.Buffer
	_, err = copied.ReadFrom(archive)
	if err != nil {
		return result, errors.Trace(err)
	}
	result.ID = args.ID
	result.Archive = ioutil.NopCloser(&copied)

	return result, nil
}
