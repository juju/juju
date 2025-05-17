// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"testing"

	"github.com/juju/tc"

	corecloud "github.com/juju/juju/core/cloud"
)

// TesOpenStackProviderTypeEqualsCoreCloudValue checks that the unique provider
// type value that the openstack provider gets registered with is equal to that
// of [corecloud.CloudTypeOpenStack].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestOpenStackProviderTypeEqualsCoreCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, corecloud.CloudTypeOpenStack.String())
}
