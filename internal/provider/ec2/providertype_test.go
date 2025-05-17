// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"testing"

	"github.com/juju/tc"

	corecloud "github.com/juju/juju/core/cloud"
)

// TestEC2ProviderTypeEqualsCoreCloudValue checks that the unique provider
// type value that the ec2 provider gets registered with is equal to that of
// [corecloud.CloudTypeEC2].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestEC2ProviderTypeEqualsCoreCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, corecloud.CloudTypeEC2.String())
}
