// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

import (
	"testing"

	"github.com/juju/tc"

	domaincloud "github.com/juju/juju/domain/cloud"
)

// TestCAASProviderTypeEqualsDomainCloudValue tests that the unique provider type
// value that a Kubernetes CAAS provider is registered with is equal to that of
// [domaincloud.CloudTypeKubernetes].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestCAASProviderTypeEqualsDomainCloudValue(t *testing.T) {
	tc.Assert(t, CAASProviderType, tc.Equals, domaincloud.CloudTypeKubernetes.String())
}
