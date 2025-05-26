// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"testing"

	"github.com/juju/tc"

	domaincloud "github.com/juju/juju/domain/cloud"
)

// TestGCEProviderTypeEqualsDomainCloudValue checks that the unique provider
// type value that the gce provider gets registered with is equal to that of
// [domaincloud.CloudTypeGCE].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestGCEProviderTypeEqualsDomainCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, domaincloud.CloudTypeGCE.String())
}
