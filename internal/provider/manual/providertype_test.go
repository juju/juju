// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"testing"

	"github.com/juju/tc"

	corecloud "github.com/juju/juju/core/cloud"
)

// TestManualProviderTypeEqualsCoreCloudValue checks that the unique provider
// type value that the manual provider gets registered with is equal to that of
// [corecloud.CloudTypeManual].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestManualProviderTypeEqualsCoreCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, corecloud.CloudTypeManual.String())
}
