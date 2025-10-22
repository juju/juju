// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unmanaged

import (
	"testing"

	"github.com/juju/tc"

	domaincloud "github.com/juju/juju/domain/cloud"
)

// TestUnmanagedProviderTypeEqualsDomainCloudValue checks that the unique provider
// type value that the unmanaged provider gets registered with is equal to that of
// [domaincloud.CloudTypeUnmanaged].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestUnmanagedProviderTypeEqualsDomainCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, domaincloud.CloudTypeUnmanaged.String())
}
