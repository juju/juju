// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"

	"github.com/juju/juju/cloud"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"path/filepath"
)

type cloudSuite struct{}

var _ = gc.Suite(&cloudSuite{})

func parsePublicClouds(c *gc.C) *cloud.Clouds {
	clouds, err := cloud.ParseCloudMetadata([]byte(cloud.FallbackPublicCloudInfo))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds.Clouds, gc.HasLen, 7)
	return clouds
}

func (s *cloudSuite) TestParseClouds(c *gc.C) {
	clouds := parsePublicClouds(c)
	var cloudNames []string
	for name, _ := range clouds.Clouds {
		cloudNames = append(cloudNames, name)
	}
	c.Assert(cloudNames, jc.SameContents,
		[]string{"aws", "aws-china", "aws-gov", "google", "azure", "azure-china", "rackspace"})
}

func (s *cloudSuite) TestParseCloudsEndpointDenormalisation(c *gc.C) {
	clouds := parsePublicClouds(c)
	rackspace := clouds.Clouds["rackspace"]
	c.Assert(rackspace.Type, gc.Equals, "openstack")
	c.Assert(rackspace.Endpoint, gc.Equals, "https://identity.api.rackspacecloud.com/v2.0")
	var regionNames []string
	for name, region := range rackspace.Regions {
		regionNames = append(regionNames, name)
		if name == "London" {
			c.Assert(region.Endpoint, gc.Equals, "https://lon.identity.api.rackspacecloud.com/v2.0")
		} else {
			c.Assert(region.Endpoint, gc.Equals, "https://identity.api.rackspacecloud.com/v2.0")
		}
	}
	c.Assert(regionNames, jc.SameContents,
		[]string{"Dallas-Fort Worth", "Chicago", "Northern Virginia", "London", "Sydney", "Hong Kong"})
}

func (s *cloudSuite) TestParseCloudsAuthTypes(c *gc.C) {
	clouds := parsePublicClouds(c)
	rackspace := clouds.Clouds["rackspace"]
	c.Assert(rackspace.AuthTypes, jc.SameContents, []cloud.AuthType{"access-key", "userpass"})
}

func (s *cloudSuite) TestPublicCloudsMetadataFallback(c *gc.C) {
	clouds, fallbackUsed, err := cloud.PublicCloudMetadata("badfile.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fallbackUsed, jc.IsTrue)
	var cloudNames []string
	for name, _ := range clouds.Clouds {
		cloudNames = append(cloudNames, name)
	}
	c.Assert(cloudNames, jc.SameContents,
		[]string{"aws", "aws-china", "aws-gov", "google", "azure", "azure-china", "rackspace"})
}

func (s *cloudSuite) TestPublicCloudsMetadata(c *gc.C) {
	metadata := `
clouds:
  aws-me:
    type: aws
    auth-types: [ userpass ]
`[1:]
	dir := c.MkDir()
	cloudyamlfile := filepath.Join(dir, "public-clouds.yaml")
	err := ioutil.WriteFile(cloudyamlfile, []byte(metadata), 0644)
	c.Assert(err, jc.ErrorIsNil)
	clouds, fallbackUsed, err := cloud.PublicCloudMetadata(cloudyamlfile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fallbackUsed, jc.IsFalse)
	c.Assert(clouds, jc.DeepEquals, &cloud.Clouds{
		Clouds: map[string]cloud.Cloud{
			"aws-me": cloud.Cloud{
				Type:      "aws",
				AuthTypes: []cloud.AuthType{"userpass"},
			},
		},
	})
}

func (s *cloudSuite) TestGeneratedPublicCloudInfo(c *gc.C) {
	cloudData, err := ioutil.ReadFile("fallback-public-cloud.yaml")
	c.Assert(err, jc.ErrorIsNil)
	clouds, err := cloud.ParseCloudMetadata(cloudData)
	c.Assert(err, jc.ErrorIsNil)

	generatedClouds := parsePublicClouds(c)
	c.Assert(clouds, jc.DeepEquals, generatedClouds)
}
