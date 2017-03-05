// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/juju/apiserver/bundle"
	"github.com/juju/juju/apiserver/params"
)

// GetBundleChanges returns the list of changes required to deploy the given
// bundle data. The changes are sorted by requirements, so that they can be
// applied in order.
// This call is deprecated, clients should use the GetChanges endpoint on the
// Bundle facade.
func (c *Client) GetBundleChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
	bundleAPI, err := bundle.NewBundle(c.api.auth)
	if err != nil {
		return params.BundleChangesResults{}, err
	}
	return bundleAPI.GetChanges(args)
}
