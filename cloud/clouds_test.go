// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
)

type cloudSuite struct{}

var _ = gc.Suite(&cloudSuite{})

var publicCloudNames = []string{
	"aws", "aws-china", "aws-gov", "google", "azure", "azure-china", "rackspace", "joyent", "cloudsigma",
}

func parsePublicClouds(c *gc.C) map[string]cloud.Cloud {
	clouds, err := cloud.ParseCloudMetadata([]byte(cloud.FallbackPublicCloudInfo))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, len(publicCloudNames))
	return clouds
}

func (s *cloudSuite) TestParseClouds(c *gc.C) {
	clouds := parsePublicClouds(c)
	var cloudNames []string
	for name, _ := range clouds {
		cloudNames = append(cloudNames, name)
	}
	c.Assert(cloudNames, jc.SameContents, publicCloudNames)
}

func (s *cloudSuite) TestParseCloudsEndpointDenormalisation(c *gc.C) {
	clouds := parsePublicClouds(c)
	rackspace := clouds["rackspace"]
	c.Assert(rackspace.Type, gc.Equals, "rackspace")
	c.Assert(rackspace.Endpoint, gc.Equals, "https://identity.api.rackspacecloud.com/v2.0")
	var regionNames []string
	for _, region := range rackspace.Regions {
		regionNames = append(regionNames, region.Name)
		if region.Name == "LON" {
			c.Assert(region.Endpoint, gc.Equals, "https://lon.identity.api.rackspacecloud.com/v2.0")
		} else {
			c.Assert(region.Endpoint, gc.Equals, "https://identity.api.rackspacecloud.com/v2.0")
		}
	}
	c.Assert(regionNames, jc.SameContents, []string{"DFW", "ORD", "IAD", "LON", "SYD", "HKG"})
}

func (s *cloudSuite) TestParseCloudsAuthTypes(c *gc.C) {
	clouds := parsePublicClouds(c)
	rackspace := clouds["rackspace"]
	c.Assert(rackspace.AuthTypes, jc.SameContents, []cloud.AuthType{"access-key", "userpass"})
}

func (s *cloudSuite) TestPublicCloudsMetadataFallback(c *gc.C) {
	clouds, fallbackUsed, err := cloud.PublicCloudMetadata("badfile.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fallbackUsed, jc.IsTrue)
	var cloudNames []string
	for name, _ := range clouds {
		cloudNames = append(cloudNames, name)
	}
	c.Assert(cloudNames, jc.SameContents, publicCloudNames)
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
	c.Assert(clouds, jc.DeepEquals, map[string]cloud.Cloud{
		"aws-me": cloud.Cloud{
			Type:      "aws",
			AuthTypes: []cloud.AuthType{"userpass"},
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
