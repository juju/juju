// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

import (
	"testing"

	"github.com/juju/tc"

	corecloud "github.com/juju/juju/core/cloud"
)

// TestCAASProviderTypeEqualsCoreCloudValue tests that the unique provider type
// value that a Kubernetes CAAS provider is registered with is equal to that of
// [corecloud.CloudTypeKubernetes].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestCAASProviderTypeEqualsCoreCloudValue(t *testing.T) {
	tc.Assert(t, CAASProviderType, tc.Equals, corecloud.CloudTypeKubernetes.String())
}
