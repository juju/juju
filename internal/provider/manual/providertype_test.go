// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"testing"

	"github.com/juju/tc"

	domaincloud "github.com/juju/juju/domain/cloud"
)

// TestManualProviderTypeEqualsDomainCloudValue checks that the unique provider
// type value that the manual provider gets registered with is equal to that of
// [domaincloud.CloudTypeManual].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestManualProviderTypeEqualsDomainCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, domaincloud.CloudTypeManual.String())
}
