// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
)

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}

var (
	GetArchFromResourceSKU = getArchFromResourceSKU
)

type CredentialInvalidator func(ctx context.Context, reason environs.CredentialInvalidReason) error

func (c CredentialInvalidator) InvalidateCredentials(ctx context.Context, reason environs.CredentialInvalidReason) error {
	return c(ctx, reason)
}
