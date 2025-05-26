// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"testing"

	"github.com/juju/tc"

	domaincloud "github.com/juju/juju/domain/cloud"
)

// TestLXDProviderTypeEqualsDomainCloudValue checks that the unique provider
// type value that the lxd provider gets registered with is equal to that of
// [domaincloud.CloudTypeLXD].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestLXDProviderTypeEqualsDomainCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, domaincloud.CloudTypeLXD.String())
}
