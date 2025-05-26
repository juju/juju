// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"testing"

	"github.com/juju/tc"

	domaincloud "github.com/juju/juju/domain/cloud"
)

// TestEC2ProviderTypeEqualsDomainCloudValue checks that the unique provider
// type value that the ec2 provider gets registered with is equal to that of
// [domaincloud.CloudTypeEC2].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestEC2ProviderTypeEqualsDomainCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, domaincloud.CloudTypeEC2.String())
}
