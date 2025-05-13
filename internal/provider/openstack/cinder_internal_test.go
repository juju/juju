// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/go-goose/goose/v5/client"
	"github.com/go-goose/goose/v5/identity"
	"github.com/juju/tc"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/testhelpers"
)

// TODO(axw) 2016-10-03 #1629721
// Change this to an external test, which will
// require refactoring the provider code to make
// it more easily testable.

type cinderInternalSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&cinderInternalSuite{})

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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(types, tc.HasLen, 1)
}

func (s *cinderInternalSuite) TestStorageProviderTypesNotSupported(c *tc.C) {
	env := &Environ{clientUnlocked: &testAuthClient{}}
	types, err := env.StorageProviderTypes()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(types, tc.HasLen, 0)
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
