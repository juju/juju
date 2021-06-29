// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/go-goose/goose/v4/client"
	"github.com/go-goose/goose/v4/identity"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

// TODO(axw) 2016-10-03 #1629721
// Change this to an external test, which will
// require refactoring the provider code to make
// it more easily testable.

type cinderInternalSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&cinderInternalSuite{})

func (s *cinderInternalSuite) TestStorageProviderTypes(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types, gc.HasLen, 1)
}

func (s *cinderInternalSuite) TestStorageProviderTypesNotSupported(c *gc.C) {
	env := &Environ{clientUnlocked: &testAuthClient{}}
	types, err := env.StorageProviderTypes()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types, gc.HasLen, 0)
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
