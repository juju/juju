// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"io/ioutil"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// UploadDirect provides the implementation of the API method.
func (b *API) UploadDirect(args params.BackupsUploadArgs) (params.BackupsMetadataResult, error) {
	var result params.BackupsMetadataResult

	if args.Data == nil {
		return result, errors.Errorf("missing archive data")
	}

	meta, err := args.Metadata.AsMetadata()
	if err != nil {
		return result, errors.Trace(err)
	}

	_, err = b.backups.Add(ioutil.NopCloser(bytes.NewBuffer(args.Data)), *meta)
	if err != nil {
		return result, errors.Trace(err)
	}

	result.UpdateFromMetadata(meta)

	return result, nil
}
