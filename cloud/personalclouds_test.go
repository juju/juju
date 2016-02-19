// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type personalCloudSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&personalCloudSuite{})

func (s *personalCloudSuite) TestWritePersonalClouds(c *gc.C) {
	clouds := map[string]cloud.Cloud{
		"homestack": cloud.Cloud{
			Type:      "openstack",
			AuthTypes: []cloud.AuthType{"userpass", "access-key"},
			Endpoint:  "http://homestack",
			Regions: []cloud.Region{
				cloud.Region{Name: "london", Endpoint: "http://london/1.0"},
			},
		},
		"azurestack": cloud.Cloud{
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
	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("clouds.yaml"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, `
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

func (s *personalCloudSuite) TestReadPersonalCloudsNone(c *gc.C) {
	clouds, err := cloud.PersonalCloudMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.IsNil)
}

func (s *personalCloudSuite) TestReadPersonalClouds(c *gc.C) {
	s.setupReadClouds(c, osenv.JujuXDGDataHomePath("clouds.yaml"))
	clouds, err := cloud.PersonalCloudMetadata()
	c.Assert(err, jc.ErrorIsNil)
	s.assertPersonalClouds(c, clouds)
}

func (s *personalCloudSuite) TestReadUserSpecifiedClouds(c *gc.C) {
	file := osenv.JujuXDGDataHomePath("somemoreclouds.yaml")
	s.setupReadClouds(c, file)
	clouds, err := cloud.ParseCloudMetadataFile(file)
	c.Assert(err, jc.ErrorIsNil)
	s.assertPersonalClouds(c, clouds)
}

func (s *personalCloudSuite) assertPersonalClouds(c *gc.C, clouds map[string]cloud.Cloud) {
	c.Assert(clouds, jc.DeepEquals, map[string]cloud.Cloud{
		"homestack": cloud.Cloud{
			Type:      "openstack",
			AuthTypes: []cloud.AuthType{"userpass", "access-key"},
			Endpoint:  "http://homestack",
			Regions: []cloud.Region{
				cloud.Region{Name: "london", Endpoint: "http://london/1.0"},
			},
		},
		"azurestack": cloud.Cloud{
			Type:            "azure",
			AuthTypes:       []cloud.AuthType{"userpass"},
			StorageEndpoint: "http://storage.azurestack.local",
			Regions: []cloud.Region{
				cloud.Region{
					Name:            "local",
					Endpoint:        "http://azurestack.local",
					StorageEndpoint: "http://storage.azurestack.local",
				},
			},
		},
	})
}

func (s *personalCloudSuite) setupReadClouds(c *gc.C, destPath string) {
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
    storage-endpoint: http://storage.azurestack.local
    regions:
      local:
        endpoint: http://azurestack.local
`[1:]
	err := ioutil.WriteFile(destPath, []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)
}
