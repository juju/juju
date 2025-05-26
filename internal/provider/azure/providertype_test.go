// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"testing"

	"github.com/juju/tc"

	domaincloud "github.com/juju/juju/domain/cloud"
)

// TestAzureProviderTypeEqualsDomainCloudValue checks that the unique provider
// type value that the azure provider gets registered with is equal to that of
// [domaincloud.CloudTypeAzure].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestAzureProviderTypeEqualsDomainCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, domaincloud.CloudTypeAzure.String())
}
