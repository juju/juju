// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/juju/cloud"
)

// CloudService provides access to clouds.
type CloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
}
