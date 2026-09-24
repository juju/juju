// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/juju/errors"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

type createParams struct {
	httprequest.Route `httprequest:"POST /backup"`
	Body              params.BackupsCreateArgs `httprequest:",body"`
}

// Create sends a request to create a backup of juju's state. The
// controller creates the archive and streams it back in the same
// response: the returned reader holds the archive bytes and must be
// closed by the caller. The backup's metadata travels in the response
// headers.
func (c *Client) Create(ctx context.Context, notes string) (params.BackupsMetadataResult, io.ReadCloser, error) {
	httpClient, err := c.st.HTTPClient(base.HTTPClientScopeUnscoped)
	if err != nil {
		return params.BackupsMetadataResult{}, nil, errors.Trace(err)
	}

	var resp *http.Response
	err = httpClient.Call(
		ctx,
		&createParams{
			Body: params.BackupsCreateArgs{
				Notes: notes,
			},
		},
		&resp,
	)
	if err != nil {
		return params.BackupsMetadataResult{}, nil, errors.Trace(apiservererrors.RestoreError(err))
	}

	var result params.BackupsMetadataResult
	if h := resp.Header.Get(params.BackupMetadataHeader); h != "" {
		if err := json.Unmarshal([]byte(h), &result); err != nil {
			_ = resp.Body.Close()
			return params.BackupsMetadataResult{}, nil, errors.Annotate(err, "decoding backup metadata")
		}
	}
	return result, resp.Body, nil
}
