// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"testing"

	"github.com/juju/tc"

	corecloud "github.com/juju/juju/core/cloud"
)

// TestOCIProviderTypeEqualsCoreCloudValue checks that the unique provider
// type value that the oci provider gets registered with is equal to that of
// [corecloud.CloudTypeOCI].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestOCIProviderTypeEqualsCoreCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, corecloud.CloudTypeOCI.String())
}
