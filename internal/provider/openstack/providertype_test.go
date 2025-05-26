// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"testing"

	"github.com/juju/tc"

	domaincloud "github.com/juju/juju/domain/cloud"
)

// TesOpenStackProviderTypeEqualsDomainCloudValue checks that the unique provider
// type value that the openstack provider gets registered with is equal to that
// of [domaincloud.CloudTypeOpenStack].
//
// This is important test to make sure that enum values are kept in sync across
// Juju.
func TestOpenStackProviderTypeEqualsDomainCloudValue(t *testing.T) {
	tc.Assert(t, providerType, tc.Equals, domaincloud.CloudTypeOpenStack.String())
}
