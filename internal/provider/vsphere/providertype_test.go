// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"testing"

	"github.com/juju/tc"

	domaincloud "github.com/juju/juju/domain/cloud"
)

// TestVSphereProviderTypeEqualsDomainCloudValue checks that the unique provider
// type value that the vsphere provider gets registered with is equal to that of
// [domaincloud.CloudTypeVSphere].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestVSphereProviderTypeEqualsDomainCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, domaincloud.CloudTypeVSphere.String())
}
