// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"testing"

	"github.com/juju/tc"

	corecloud "github.com/juju/juju/core/cloud"
)

// TestVSphereProviderTypeEqualsCoreCloudValue checks that the unique provider
// type value that the vsphere provider gets registered with is equal to that of
// [corecloud.CloudTypevSphere].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestVSphereProviderTypeEqualsCoreCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, corecloud.CloudTypevSphere.String())
}
