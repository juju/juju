// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"testing"

	"github.com/juju/tc"

	corecloud "github.com/juju/juju/core/cloud"
)

// TestAzureProviderTypeEqualsCoreCloudValue checks that the unique provider
// type value that the azure provider gets registered with is equal to that of
// [corecloud.CloudTypeAzure].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestAzureProviderTypeEqualsCoreCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, corecloud.CloudTypeAzure.String())
}
