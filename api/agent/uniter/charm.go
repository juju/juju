// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"fmt"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

// This module implements a subset of the interface provided by
// state.Charm, as needed by the uniter API.

// Charm represents the state of a charm in the model.
type Charm struct {
	client *Client
	curl   string
}

// URL returns the URL that identifies the charm.
func (c *Charm) URL() string {
	return c.curl
}

// ArchiveSha256 returns the SHA256 digest of the charm archive
// (bundle) bytes.
//
// NOTE: This differs from state.Charm.BundleSha256() by returning an
// error as well, because it needs to make an API call. It's also
// renamed to avoid confusion with juju deployment bundles.
//
// TODO(dimitern): 2013-09-06 bug 1221834
// Cache the result after getting it once for the same charm URL,
// because it's immutable.
func (c *Charm) ArchiveSha256(ctx context.Context) (string, error) {
	var results params.StringResults
	args := params.CharmURLs{
		URLs: []params.CharmURL{{URL: c.curl}},
	}
	err := c.client.facade.FacadeCall(ctx, "CharmArchiveSha256", args, &results)
	if err != nil {
		return "", err
	}
	if len(results.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", apiservererrors.RestoreError(result.Error)
	}
	return result.Result, nil
}