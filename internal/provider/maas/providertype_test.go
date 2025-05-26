// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"testing"

	"github.com/juju/tc"

	domaincloud "github.com/juju/juju/domain/cloud"
)

// TestMAASProviderTypeEqualsDomainCloudValue checks that the unique provider
// type value that the maas provider gets registered with is equal to that of
// [domaincloud.CloudTypeMAAS].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestMAASProviderTypeEqualsDomainCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, domaincloud.CloudTypeMAAS.String())
}
