// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// UploadDirect provides the implementation of the API method.
func (a *API) UploadDirect(args params.BackupsUploadArgs) (params.BackupsMetadataResult, error) {
	var result params.BackupsMetadataResult

	backupsMethods, closer := newBackups(a.st)
	defer closer.Close()

	if args.Data == nil {
		return result, errors.Errorf("missing archive data")
	}

	meta := MetadataFromResult(args.Metadata)
	if _, err := backupsMethods.Add(bytes.NewBuffer(args.Data), meta); err != nil {
		return result, errors.Trace(err)
	}

	return ResultFromMetadata(meta), nil
}
