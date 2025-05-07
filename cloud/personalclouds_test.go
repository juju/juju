// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"os"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
)

type personalCloudSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = tc.Suite(&personalCloudSuite{})

func (s *personalCloudSuite) TestWritePersonalClouds(c *tc.C) {
	clouds := map[string]cloud.Cloud{
		"homestack": {
			Type:      "openstack",
			AuthTypes: []cloud.AuthType{"userpass", "access-key"},
			Endpoint:  "http://homestack",
			Regions: []cloud.Region{
				{Name: "london", Endpoint: "http://london/1.0"},
			},
		},
		"azurestack": {
			Type:      "azure",
			AuthTypes: []cloud.AuthType{"userpass"},
			Regions: []cloud.Region{{
				Name:     "prod",
				Endpoint: "http://prod.azurestack.local",
			}, {
				Name:     "dev",
				Endpoint: "http://dev.azurestack.local",
			}, {
				Name:     "test",
				Endpoint: "http://test.azurestack.local",
			}},
		},
	}
	err := cloud.WritePersonalCloudMetadata(clouds)
	c.Assert(err, jc.ErrorIsNil)
	data, err := os.ReadFile(osenv.JujuXDGDataHomePath("clouds.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, `
clouds:
  azurestack:
    type: azure
    auth-types: [userpass]
    regions:
      prod:
        endpoint: http://prod.azurestack.local
      dev:
        endpoint: http://dev.azurestack.local
      test:
        endpoint: http://test.azurestack.local
  homestack:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
`[1:])
}

func (s *personalCloudSuite) TestReadPersonalCloudsNone(c *tc.C) {
	clouds, err := cloud.PersonalCloudMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, tc.IsNil)
}

func (s *personalCloudSuite) TestReadPersonalClouds(c *tc.C) {
	s.setupReadClouds(c, osenv.JujuXDGDataHomePath("clouds.yaml"))
	clouds, err := cloud.PersonalCloudMetadata()
	c.Assert(err, jc.ErrorIsNil)
	s.assertPersonalClouds(c, clouds)
}

func (s *personalCloudSuite) TestReadUserSpecifiedClouds(c *tc.C) {
	file := osenv.JujuXDGDataHomePath("somemoreclouds.yaml")
	s.setupReadClouds(c, file)
	clouds, err := cloud.ParseCloudMetadataFile(file)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPersonalClouds(c, clouds)
}

func (s *personalCloudSuite) assertPersonalClouds(c *tc.C, clouds map[string]cloud.Cloud) {
	c.Assert(clouds, jc.DeepEquals, map[string]cloud.Cloud{
		"homestack": {
			Name:        "homestack",
			Type:        "openstack",
			Description: "Openstack Cloud",
			AuthTypes:   []cloud.AuthType{"userpass", "access-key"},
			Endpoint:    "http://homestack",
			Regions: []cloud.Region{
				{Name: "london", Endpoint: "http://london/1.0"},
			},
		},
		"azurestack": {
			Name:             "azurestack",
			Type:             "azure",
			Description:      "Microsoft Azure",
			AuthTypes:        []cloud.AuthType{"userpass"},
			IdentityEndpoint: "http://login.azurestack.local",
			StorageEndpoint:  "http://storage.azurestack.local",
			Regions: []cloud.Region{
				{
					Name:             "local",
					Endpoint:         "http://azurestack.local",
					IdentityEndpoint: "http://login.azurestack.local",
					StorageEndpoint:  "http://storage.azurestack.local",
				},
			},
		},
	})
}

func (s *personalCloudSuite) setupReadClouds(c *tc.C, destPath string) {
	data := `
clouds:
  homestack:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
  azurestack:
    type: azure
    auth-types: [userpass]
    identity-endpoint: http://login.azurestack.local
    storage-endpoint: http://storage.azurestack.local
    regions:
      local:
        endpoint: http://azurestack.local
`[1:]
	err := os.WriteFile(destPath, []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)
}
