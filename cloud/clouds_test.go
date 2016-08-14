// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/testing"
)

type cloudSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

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
		if region.Name == "lon" {
			c.Assert(region.Endpoint, gc.Equals, "https://lon.identity.api.rackspacecloud.com/v2.0")
		} else {
			c.Assert(region.Endpoint, gc.Equals, "https://identity.api.rackspacecloud.com/v2.0")
		}
	}
	c.Assert(regionNames, jc.SameContents, []string{"dfw", "ord", "iad", "lon", "syd", "hkg"})
}

func (s *cloudSuite) TestParseCloudsAuthTypes(c *gc.C) {
	clouds := parsePublicClouds(c)
	rackspace := clouds["rackspace"]
	c.Assert(rackspace.AuthTypes, jc.SameContents, cloud.AuthTypes{"access-key", "userpass"})
}

func (s *cloudSuite) TestParseCloudsConfig(c *gc.C) {
	clouds, err := cloud.ParseCloudMetadata([]byte(`clouds:
  testing:
    type: dummy
    config:
      k1: v1
      k2: 2.0
`))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 1)
	testingCloud := clouds["testing"]
	c.Assert(testingCloud, jc.DeepEquals, cloud.Cloud{
		Type: "dummy",
		Config: map[string]interface{}{
			"k1": "v1",
			"k2": float64(2.0),
		},
	})
}

func (s *cloudSuite) TestParseCloudsRegionConfig(c *gc.C) {
	clouds, err := cloud.ParseCloudMetadata([]byte(`clouds:
  testing:
    type: dummy
    config:
      k1: v1
      k2: 2.0
    region-config:
      region1:
        mascot: [eggs, ham]
      region2:
        mascot: glenda
      region3:
        mascot:  gopher
`))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 1)
	testingCloud := clouds["testing"]
	c.Assert(testingCloud, jc.DeepEquals, cloud.Cloud{
		Type: "dummy",
		Config: map[string]interface{}{
			"k1": "v1",
			"k2": float64(2.0),
		},
		RegionConfig: cloud.RegionConfig{
			"region1": cloud.Attrs{
				"mascot": []interface{}{"eggs", "ham"},
			},

			"region2": cloud.Attrs{
				"mascot": "glenda",
			},

			"region3": cloud.Attrs{
				"mascot": "gopher",
			},
		},
	})
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

func (s *cloudSuite) TestWritePublicCloudsMetadata(c *gc.C) {
	clouds := map[string]cloud.Cloud{
		"aws-me": cloud.Cloud{
			Type:      "aws",
			AuthTypes: []cloud.AuthType{"userpass"},
		},
	}
	err := cloud.WritePublicCloudMetadata(clouds)
	c.Assert(err, jc.ErrorIsNil)
	publicClouds, fallbackUsed, err := cloud.PublicCloudMetadata(cloud.JujuPublicCloudsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fallbackUsed, jc.IsFalse)
	c.Assert(publicClouds, jc.DeepEquals, clouds)
}

func (s *cloudSuite) assertCompareClouds(c *gc.C, meta2 string, expected bool) {
	meta1 := `
clouds:
  aws-me:
    type: aws
    auth-types: [ userpass ]
`[1:]
	if meta2 == "" {
		meta2 = meta1
	}
	c1, err := cloud.ParseCloudMetadata([]byte(meta1))
	c.Assert(err, jc.ErrorIsNil)
	c2, err := cloud.ParseCloudMetadata([]byte(meta2))
	c.Assert(err, jc.ErrorIsNil)
	result, err := cloud.IsSameCloudMetadata(c1, c2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, expected)
}

func (s *cloudSuite) TestIsSameCloudsMetadataSameData(c *gc.C) {
	s.assertCompareClouds(c, "", true)
}

func (s *cloudSuite) TestIsSameCloudsMetadataExistingCloudChanged(c *gc.C) {
	metadata := `
clouds:
  aws-me:
    type: aws
    auth-types: [ userpass ]
    endpoint: http://endpoint
`[1:]
	s.assertCompareClouds(c, metadata, false)
}

func (s *cloudSuite) TestIsSameCloudsMetadataNewCloudAdded(c *gc.C) {
	metadata := `
clouds:
  aws-me:
    type: aws
    auth-types: [ userpass ]
  gce-me:
    type: gce
    auth-types: [ userpass ]
`[1:]
	s.assertCompareClouds(c, metadata, false)
}

func (s *cloudSuite) TestRegionNames(c *gc.C) {
	regions := []cloud.Region{
		{Name: "mars"},
		{Name: "earth"},
		{Name: "jupiter"},
	}

	names := cloud.RegionNames(regions)
	c.Assert(names, gc.DeepEquals, []string{"earth", "jupiter", "mars"})

	c.Assert(cloud.RegionNames([]cloud.Region{}), gc.HasLen, 0)
	c.Assert(cloud.RegionNames(nil), gc.HasLen, 0)
}
