// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
)


var (
	GetArchFromResourceSKU = getArchFromResourceSKU
)

type CredentialInvalidator func(ctx context.Context, reason environs.CredentialInvalidReason) error

func (c CredentialInvalidator) InvalidateCredentials(ctx context.Context, reason environs.CredentialInvalidReason) error {
	return c(ctx, reason)
}
