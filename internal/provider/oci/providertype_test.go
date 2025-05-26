// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"testing"

	"github.com/juju/tc"

	domaincloud "github.com/juju/juju/domain/cloud"
)

// TestOCIProviderTypeEqualsDomainCloudValue checks that the unique provider
// type value that the oci provider gets registered with is equal to that of
// [domaincloud.CloudTypeOCI].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestOCIProviderTypeEqualsDomainCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, domaincloud.CloudTypeOCI.String())
}
