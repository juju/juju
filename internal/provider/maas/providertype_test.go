// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"testing"

	"github.com/juju/tc"

	corecloud "github.com/juju/juju/core/cloud"
)

// TestMAASProviderTypeEqualsCoreCloudValue checks that the unique provider
// type value that the maas provider gets registered with is equal to that of
// [corecloud.CloudTypeMAAS].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestMAASProviderTypeEqualsCoreCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, corecloud.CloudTypeMAAS.String())
}
