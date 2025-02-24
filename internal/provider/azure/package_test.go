// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

var (
	GetArchFromResourceSKU = getArchFromResourceSKU
)

type CredentialInvalidator func(ctx context.Context, reason environs.CredentialInvalidReason) error

func (c CredentialInvalidator) InvalidateCredentials(ctx context.Context, reason environs.CredentialInvalidReason) error {
	return c(ctx, reason)
}
