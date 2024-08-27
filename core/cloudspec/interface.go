// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec

import (
	"context"

	"github.com/juju/juju/environs/cloudspec"
)

type CloudSpecGetter interface {
	CloudSpecForModel(ctx context.Context, namespace string) (cloudspec.CloudSpec, error)
}
