// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"testing"

	"github.com/juju/tc"

	corecloud "github.com/juju/juju/core/cloud"
)

// TestLXDProviderTypeEqualsCoreCloudValue checks that the unique provider
// type value that the lxd provider gets registered with is equal to that of
// [corecloud.CloudTypeLXD].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestLXDProviderTypeEqualsCoreCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, corecloud.CloudTypeLXD.String())
}
