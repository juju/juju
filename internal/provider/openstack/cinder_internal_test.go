// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"testing"

	"github.com/go-goose/goose/v5/client"
	"github.com/go-goose/goose/v5/identity"
	"github.com/juju/tc"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
)

// TODO(axw) 2016-10-03 #1629721
// Change this to an external test, which will
// require refactoring the provider code to make
// it more easily testable.

type cinderInternalSuite struct {
	testhelpers.IsolationSuite
}

func TestCinderInternalSuite(t *testing.T) {
	tc.Run(t, &cinderInternalSuite{})
}

func (s *cinderInternalSuite) TestStorageProviderTypes(c *tc.C) {
	env := &Environ{
		cloudUnlocked: environscloudspec.CloudSpec{
			Region: "foo",
		},
		clientUnlocked: &testAuthClient{
			regionEndpoints: map[string]identity.ServiceURLs{
				"foo": {"volumev2": "https://bar.invalid"},
			},
		}}
	types, err := env.StorageProviderTypes()
	c.Check(err, tc.ErrorIsNil)
	c.Check(types, tc.SameContents, []storage.ProviderType{
		"cinder",
		"loop",
		"tmpfs",
		"rootfs",
	})
}

// TestStorageProviderTypesNotSupported tests that when the environ does not
// support Cinder storage it does not come out as one of the storage provider
// types available.
func (s *cinderInternalSuite) TestStorageProviderTypesNotSupported(c *tc.C) {
	env := &Environ{clientUnlocked: &testAuthClient{}}
	types, err := env.StorageProviderTypes()
	c.Check(err, tc.ErrorIsNil)
	c.Check(types, tc.SameContents, []storage.ProviderType{
		"loop",
		"tmpfs",
		"rootfs",
	})
}

type testAuthClient struct {
	client.AuthenticatingClient
	regionEndpoints map[string]identity.ServiceURLs
}

func (r *testAuthClient) IsAuthenticated() bool {
	return true
}

func (r *testAuthClient) TenantId() string {
	return "tenant-id"
}

func (r *testAuthClient) EndpointsForRegion(region string) identity.ServiceURLs {
	return r.regionEndpoints[region]
}
