// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"context"
)

// API is a dummy struct for compatibility.
type API struct{}

// UpdateFromPublishedImages is now a no-op.
// It is retained for compatibility.
func (api *API) UpdateFromPublishedImages(ctx context.Context) error {
	return nil
}
