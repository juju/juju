// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/juju/errors"
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
	"aws", "aws-china", "aws-gov", "equinix", "google", "azure", "azure-china", "oracle",
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
	for name := range clouds {
		cloudNames = append(cloudNames, name)
	}
	c.Assert(cloudNames, jc.SameContents, publicCloudNames)
}

func (s *cloudSuite) TestAuthTypesContains(c *gc.C) {
	ats := cloud.AuthTypes{"a1", "a2"}
	c.Assert(ats.Contains(cloud.AuthType("unknown")), jc.IsFalse)
	c.Assert(ats.Contains(cloud.AuthType("a1")), jc.IsTrue)
	c.Assert(ats.Contains(cloud.AuthType("a2")), jc.IsTrue)
}

func (s *cloudSuite) TestParseCloudsEndpointDenormalisation(c *gc.C) {
	clouds := parsePublicClouds(c)
	oracle := clouds["oracle"]
	c.Assert(oracle.Type, gc.Equals, "oci")
	var regionNames []string
	for _, region := range oracle.Regions {
		regionNames = append(regionNames, region.Name)
		endpointURL := fmt.Sprintf("https://iaas.%s.oraclecloud.com", region.Name)
		c.Assert(region.Endpoint, gc.Equals, endpointURL)
	}
	c.Assert(regionNames, jc.SameContents, []string{"af-johannesburg-1", "ap-chiyoda-1", "ap-chuncheon-1", "ap-dcc-canberra-1", "ap-hyderabad-1", "ap-ibaraki-1", "ap-melbourne-1", "ap-mumbai-1", "ap-osaka-1", "ap-seoul-1", "ap-singapore-1", "ap-sydney-1", "ap-tokyo-1", "ca-montreal-1", "ca-toronto-1", "eu-amsterdam-1", "eu-dcc-dublin-1", "eu-dcc-dublin-2", "eu-dcc-milan-1", "eu-dcc-milan-2", "eu-dcc-rating-1", "eu-dcc-rating-2", "eu-frankfurt-1", "eu-frankfurt-2", "eu-jovanovac-1", "eu-madrid-1", "eu-madrid-2", "eu-marseille-1", "eu-milan-1", "eu-paris-1", "eu-stockholm-1", "eu-zurich-1", "il-jerusalem-1", "me-abudhabi-1", "me-dcc-muscat-1", "me-dubai-1", "me-jeddah-1", "mx-monterrey-1", "mx-queretaro-1", "sa-santiago-1", "sa-saopaulo-1", "sa-vinhedo-1", "uk-cardiff-1", "uk-london-1", "us-ashburn-1", "us-chicago-1", "us-langley-1", "us-luke-1", "us-phoenix-1", "us-sanjose-1"})
}

func (s *cloudSuite) TestParseCloudsAuthTypes(c *gc.C) {
	clouds := parsePublicClouds(c)
	equinix := clouds["equinix"]
	c.Assert(equinix.AuthTypes, jc.SameContents, cloud.AuthTypes{"access-key"})
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
		Name: "testing",
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
		Name: "testing",
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

func (s *cloudSuite) TestParseCloudsIgnoresNameField(c *gc.C) {
	clouds, err := cloud.ParseCloudMetadata([]byte(`clouds:
  testing:
    name: ignored
    type: dummy
`))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, gc.HasLen, 1)
	testingCloud := clouds["testing"]
	c.Assert(testingCloud, jc.DeepEquals, cloud.Cloud{
		Name: "testing",
		Type: "dummy",
	})
}

func (s *cloudSuite) TestPublicCloudsMetadataFallback(c *gc.C) {
	clouds, fallbackUsed, err := cloud.PublicCloudMetadata("badfile.yaml")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fallbackUsed, jc.IsTrue)
	var cloudNames []string
	for name := range clouds {
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
	err := os.WriteFile(cloudyamlfile, []byte(metadata), 0644)
	c.Assert(err, jc.ErrorIsNil)
	clouds, fallbackUsed, err := cloud.PublicCloudMetadata(cloudyamlfile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fallbackUsed, jc.IsFalse)
	c.Assert(clouds, jc.DeepEquals, map[string]cloud.Cloud{
		"aws-me": {
			Name:        "aws-me",
			Type:        "aws",
			Description: "Amazon Web Services",
			AuthTypes:   []cloud.AuthType{"userpass"},
		},
	})
}

func (s *cloudSuite) TestGeneratedPublicCloudInfo(c *gc.C) {
	cloudData, err := os.ReadFile("fallback-public-cloud.yaml")
	c.Assert(err, jc.ErrorIsNil)
	clouds, err := cloud.ParseCloudMetadata(cloudData)
	c.Assert(err, jc.ErrorIsNil)

	generatedClouds := parsePublicClouds(c)
	c.Assert(clouds, jc.DeepEquals, generatedClouds)
}

func (s *cloudSuite) TestWritePublicCloudsMetadata(c *gc.C) {
	clouds := map[string]cloud.Cloud{
		"aws-me": {
			Name:        "aws-me",
			Type:        "aws",
			Description: "Amazon Web Services",
			AuthTypes:   []cloud.AuthType{"userpass"},
		},
	}
	err := cloud.WritePublicCloudMetadata(clouds)
	c.Assert(err, jc.ErrorIsNil)
	publicClouds, fallbackUsed, err := cloud.PublicCloudMetadata(cloud.JujuPublicCloudsPath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fallbackUsed, jc.IsFalse)
	c.Assert(publicClouds, jc.DeepEquals, clouds)
}

func (s *cloudSuite) TestWritePublicCloudsMetadataCloudNameIgnored(c *gc.C) {
	awsMeCloud := cloud.Cloud{
		Name:        "ignored",
		Type:        "aws",
		Description: "Amazon Web Services",
		AuthTypes:   []cloud.AuthType{"userpass"},
	}
	clouds := map[string]cloud.Cloud{"aws-me": awsMeCloud}
	err := cloud.WritePublicCloudMetadata(clouds)
	c.Assert(err, jc.ErrorIsNil)
	publicClouds, _, err := cloud.PublicCloudMetadata(cloud.JujuPublicCloudsPath())
	c.Assert(err, jc.ErrorIsNil)

	// The clouds.yaml file does not include the cloud name as a property
	// of the cloud object, so the cloud.Name field is ignored. On the way
	// out it should be set to the same as the map key.
	awsMeCloud.Name = "aws-me"
	clouds["aws-me"] = awsMeCloud
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

func (s *cloudSuite) TestMarshalCloud(c *gc.C) {
	in := cloud.Cloud{
		Name:           "foo",
		Type:           "bar",
		AuthTypes:      []cloud.AuthType{"baz"},
		Endpoint:       "qux",
		CACertificates: []string{"fakecacert"},
		SkipTLSVerify:  true,
	}
	marshalled, err := cloud.MarshalCloud(in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(marshalled), gc.Equals, `
name: foo
type: bar
auth-types: [baz]
endpoint: qux
ca-certificates:
- fakecacert
skip-tls-verify: true
`[1:])
}

func (s *cloudSuite) TestUnmarshalCloud(c *gc.C) {
	in := []byte(`
name: foo
type: bar
auth-types: [baz]
endpoint: qux
ca-certificates: [fakecacert]
skip-tls-verify: true
`)
	out, err := cloud.UnmarshalCloud(in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, cloud.Cloud{
		Name:           "foo",
		Type:           "bar",
		AuthTypes:      []cloud.AuthType{"baz"},
		Endpoint:       "qux",
		CACertificates: []string{"fakecacert"},
		SkipTLSVerify:  true,
	})
}

func (s *cloudSuite) TestRegionByNameNoRegions(c *gc.C) {
	r, err := cloud.RegionByName([]cloud.Region{}, "star")
	c.Assert(r, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`region "star" not found (cloud has no regions)`))
}

func (s *cloudSuite) TestRegionByName(c *gc.C) {
	regions := []cloud.Region{
		{Name: "mars"},
		{Name: "earth"},
		{Name: "jupiter"},
	}

	r, err := cloud.RegionByName(regions, "mars")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r, gc.Not(gc.IsNil))
	c.Assert(r, gc.DeepEquals, &cloud.Region{Name: "mars"})
}

func (s *cloudSuite) TestRegionByNameNotFound(c *gc.C) {
	regions := []cloud.Region{
		{Name: "mars"},
		{Name: "earth"},
		{Name: "jupiter"},
	}

	r, err := cloud.RegionByName(regions, "star")
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`region "star" not found (expected one of ["earth" "jupiter" "mars"])`))
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(r, gc.IsNil)
}
