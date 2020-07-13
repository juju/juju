// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// API is a dummy struct for compatibility.
type API struct{}

// NewAPI returns a new cloud image metadata API facade.
func NewAPI(ctx facade.Context) (*API, error) {
	if !ctx.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	return &API{}, nil
}

// UpdateFromPublishedImages is now a no-op.
// It is retained for compatibility.
func (api *API) UpdateFromPublishedImages() error {
	return nil
}
