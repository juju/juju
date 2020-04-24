// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/facades/client/bundle"
	"github.com/juju/juju/apiserver/params"
)

// GetBundleChanges returns the list of changes required to deploy the given
// bundle data. The changes are sorted by requirements, so that they can be
// applied in order.
// This call is deprecated, clients should use the GetChanges endpoint on the
// Bundle facade.
// Note: any new feature in the future like devices will never be supported here.
func (c *Client) GetBundleChanges(args params.BundleChangesParams) (params.BundleChangesResults, error) {
	st := c.api.state()
	apiV1, err := bundle.NewBundleAPIv1(bundle.NewStateShim(st), c.api.auth, names.NewModelTag(st.ModelUUID()))
	if err != nil {
		return params.BundleChangesResults{}, err
	}
	return apiV1.GetChanges(args)
}
