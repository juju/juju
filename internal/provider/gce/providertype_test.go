// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"testing"

	"github.com/juju/tc"

	corecloud "github.com/juju/juju/core/cloud"
)

// TestGCEProviderTypeEqualsCoreCloudValue checks that the unique provider
// type value that the gce provider gets registered with is equal to that of
// [corecloud.CloudTypeGCE].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestGCEProviderTypeEqualsCoreCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, corecloud.CloudTypeGCE.String())
}
