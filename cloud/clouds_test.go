// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/internal/testing"
)

type cloudSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = tc.Suite(&cloudSuite{})

var publicCloudNames = []string{
	"aws", "aws-china", "aws-gov", "google", "azure", "azure-china", "oracle",
}

func parsePublicClouds(c *tc.C) map[string]cloud.Cloud {
	clouds, err := cloud.ParseCloudMetadata([]byte(cloud.FallbackPublicCloudInfo))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, len(publicCloudNames))
	return clouds
}

func (s *cloudSuite) TestParseClouds(c *tc.C) {
	clouds := parsePublicClouds(c)
	var cloudNames []string
	for name := range clouds {
		cloudNames = append(cloudNames, name)
	}
	c.Assert(cloudNames, tc.SameContents, publicCloudNames)
}

func (s *cloudSuite) TestAuthTypesContains(c *tc.C) {
	ats := cloud.AuthTypes{"a1", "a2"}
	c.Assert(ats.Contains(cloud.AuthType("unknown")), tc.IsFalse)
	c.Assert(ats.Contains(cloud.AuthType("a1")), tc.IsTrue)
	c.Assert(ats.Contains(cloud.AuthType("a2")), tc.IsTrue)
}

func (s *cloudSuite) TestParseCloudsEndpointDenormalisation(c *tc.C) {
	clouds := parsePublicClouds(c)
	oracle := clouds["oracle"]
	c.Assert(oracle.Type, tc.Equals, "oci")
	var regionNames []string
	for _, region := range oracle.Regions {
		regionNames = append(regionNames, region.Name)
		endpointURL := fmt.Sprintf("https://iaas.%s.oraclecloud.com", region.Name)
		c.Assert(region.Endpoint, tc.Equals, endpointURL)
	}
	c.Assert(regionNames, tc.SameContents, []string{"af-johannesburg-1", "ap-chiyoda-1", "ap-chuncheon-1", "ap-dcc-canberra-1", "ap-hyderabad-1", "ap-ibaraki-1", "ap-melbourne-1", "ap-mumbai-1", "ap-osaka-1", "ap-seoul-1", "ap-singapore-1", "ap-sydney-1", "ap-tokyo-1", "ca-montreal-1", "ca-toronto-1", "eu-amsterdam-1", "eu-dcc-dublin-1", "eu-dcc-dublin-2", "eu-dcc-milan-1", "eu-dcc-milan-2", "eu-dcc-rating-1", "eu-dcc-rating-2", "eu-frankfurt-1", "eu-frankfurt-2", "eu-jovanovac-1", "eu-madrid-1", "eu-madrid-2", "eu-marseille-1", "eu-milan-1", "eu-paris-1", "eu-stockholm-1", "eu-zurich-1", "il-jerusalem-1", "me-abudhabi-1", "me-dcc-muscat-1", "me-dubai-1", "me-jeddah-1", "mx-monterrey-1", "mx-queretaro-1", "sa-santiago-1", "sa-saopaulo-1", "sa-vinhedo-1", "uk-cardiff-1", "uk-london-1", "us-ashburn-1", "us-chicago-1", "us-langley-1", "us-luke-1", "us-phoenix-1", "us-sanjose-1"})
}

func (s *cloudSuite) TestParseCloudsAuthTypes(c *tc.C) {
	clouds := parsePublicClouds(c)
	google := clouds["google"]
	c.Assert(google.AuthTypes, tc.SameContents, cloud.AuthTypes{"jsonfile", "oauth2"})
}

func (s *cloudSuite) TestParseCloudsConfig(c *tc.C) {
	clouds, err := cloud.ParseCloudMetadata([]byte(`clouds:
  testing:
    type: dummy
    config:
      k1: v1
      k2: 2.0
`))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, 1)
	testingCloud := clouds["testing"]
	c.Assert(testingCloud, tc.DeepEquals, cloud.Cloud{
		Name: "testing",
		Type: "dummy",
		Config: map[string]interface{}{
			"k1": "v1",
			"k2": float64(2.0),
		},
	})
}

func (s *cloudSuite) TestParseCloudsRegionConfig(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, 1)
	testingCloud := clouds["testing"]
	c.Assert(testingCloud, tc.DeepEquals, cloud.Cloud{
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

func (s *cloudSuite) TestParseCloudsIgnoresNameField(c *tc.C) {
	clouds, err := cloud.ParseCloudMetadata([]byte(`clouds:
  testing:
    name: ignored
    type: dummy
`))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(clouds, tc.HasLen, 1)
	testingCloud := clouds["testing"]
	c.Assert(testingCloud, tc.DeepEquals, cloud.Cloud{
		Name: "testing",
		Type: "dummy",
	})
}

func (s *cloudSuite) TestPublicCloudsMetadataFallback(c *tc.C) {
	clouds, fallbackUsed, err := cloud.PublicCloudMetadata("badfile.yaml")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fallbackUsed, tc.IsTrue)
	var cloudNames []string
	for name := range clouds {
		cloudNames = append(cloudNames, name)
	}
	c.Assert(cloudNames, tc.SameContents, publicCloudNames)
}

func (s *cloudSuite) TestPublicCloudsMetadata(c *tc.C) {
	metadata := `
clouds:
  aws-me:
    type: aws
    auth-types: [ userpass ]
`[1:]
	dir := c.MkDir()
	cloudyamlfile := filepath.Join(dir, "public-clouds.yaml")
	err := os.WriteFile(cloudyamlfile, []byte(metadata), 0644)
	c.Assert(err, tc.ErrorIsNil)
	clouds, fallbackUsed, err := cloud.PublicCloudMetadata(cloudyamlfile)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fallbackUsed, tc.IsFalse)
	c.Assert(clouds, tc.DeepEquals, map[string]cloud.Cloud{
		"aws-me": {
			Name:        "aws-me",
			Type:        "aws",
			Description: "Amazon Web Services",
			AuthTypes:   []cloud.AuthType{"userpass"},
		},
	})
}

func (s *cloudSuite) TestAzurePublicCloudsMetadata(c *tc.C) {
	metadata := `
clouds:
  azure-me:
    type: azure
    auth-types: [ service-principal-secret ]
`[1:]
	dir := c.MkDir()
	cloudyamlfile := filepath.Join(dir, "public-clouds.yaml")
	err := os.WriteFile(cloudyamlfile, []byte(metadata), 0644)
	c.Assert(err, tc.ErrorIsNil)
	clouds, fallbackUsed, err := cloud.PublicCloudMetadata(cloudyamlfile)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fallbackUsed, tc.IsFalse)
	c.Assert(clouds, tc.DeepEquals, map[string]cloud.Cloud{
		"azure-me": {
			Name:        "azure-me",
			Type:        "azure",
			Description: "Microsoft Azure",
			AuthTypes:   []cloud.AuthType{"service-principal-secret", "managed-identity"},
		},
	})
}

func (s *cloudSuite) TestGeneratedPublicCloudInfo(c *tc.C) {
	cloudData, err := os.ReadFile("fallback-public-cloud.yaml")
	c.Assert(err, tc.ErrorIsNil)
	clouds, err := cloud.ParseCloudMetadata(cloudData)
	c.Assert(err, tc.ErrorIsNil)

	generatedClouds := parsePublicClouds(c)
	c.Assert(clouds, tc.DeepEquals, generatedClouds)
}

func (s *cloudSuite) TestWritePublicCloudsMetadata(c *tc.C) {
	clouds := map[string]cloud.Cloud{
		"aws-me": {
			Name:        "aws-me",
			Type:        "aws",
			Description: "Amazon Web Services",
			AuthTypes:   []cloud.AuthType{"userpass"},
		},
	}
	err := cloud.WritePublicCloudMetadata(clouds)
	c.Assert(err, tc.ErrorIsNil)
	publicClouds, fallbackUsed, err := cloud.PublicCloudMetadata(cloud.JujuPublicCloudsPath())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(fallbackUsed, tc.IsFalse)
	c.Assert(publicClouds, tc.DeepEquals, clouds)
}

func (s *cloudSuite) TestWritePublicCloudsMetadataCloudNameIgnored(c *tc.C) {
	awsMeCloud := cloud.Cloud{
		Name:        "ignored",
		Type:        "aws",
		Description: "Amazon Web Services",
		AuthTypes:   []cloud.AuthType{"userpass"},
	}
	clouds := map[string]cloud.Cloud{"aws-me": awsMeCloud}
	err := cloud.WritePublicCloudMetadata(clouds)
	c.Assert(err, tc.ErrorIsNil)
	publicClouds, _, err := cloud.PublicCloudMetadata(cloud.JujuPublicCloudsPath())
	c.Assert(err, tc.ErrorIsNil)

	// The clouds.yaml file does not include the cloud name as a property
	// of the cloud object, so the cloud.Name field is ignored. On the way
	// out it should be set to the same as the map key.
	awsMeCloud.Name = "aws-me"
	clouds["aws-me"] = awsMeCloud
	c.Assert(publicClouds, tc.DeepEquals, clouds)
}

func (s *cloudSuite) assertCompareClouds(c *tc.C, meta2 string, expected bool) {
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
	c.Assert(err, tc.ErrorIsNil)
	c2, err := cloud.ParseCloudMetadata([]byte(meta2))
	c.Assert(err, tc.ErrorIsNil)
	result, err := cloud.IsSameCloudMetadata(c1, c2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, expected)
}

func (s *cloudSuite) TestIsSameCloudsMetadataSameData(c *tc.C) {
	s.assertCompareClouds(c, "", true)
}

func (s *cloudSuite) TestIsSameCloudsMetadataExistingCloudChanged(c *tc.C) {
	metadata := `
clouds:
  aws-me:
    type: aws
    auth-types: [ userpass ]
    endpoint: http://endpoint
`[1:]
	s.assertCompareClouds(c, metadata, false)
}

func (s *cloudSuite) TestIsSameCloudsMetadataNewCloudAdded(c *tc.C) {
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

func (s *cloudSuite) TestMalformedYAMLNoPanic(_ *tc.C) {
	// Note the bad indentation. This case was reported under LP:2039322.
	metadata := `
clouds:
manual-cloud:
    type: manual
    endpoint: ubuntu@some-host-fqdn
`[1:]

	// We don't care about the result, just that there is no panic.
	_, _ = cloud.ParseCloudMetadata([]byte(metadata))
}

func (s *cloudSuite) TestRegionNames(c *tc.C) {
	regions := []cloud.Region{
		{Name: "mars"},
		{Name: "earth"},
		{Name: "jupiter"},
	}

	names := cloud.RegionNames(regions)
	c.Assert(names, tc.DeepEquals, []string{"earth", "jupiter", "mars"})

	c.Assert(cloud.RegionNames([]cloud.Region{}), tc.HasLen, 0)
	c.Assert(cloud.RegionNames(nil), tc.HasLen, 0)
}

func (s *cloudSuite) TestMarshalCloud(c *tc.C) {
	in := cloud.Cloud{
		Name:           "foo",
		Type:           "bar",
		AuthTypes:      []cloud.AuthType{"baz"},
		Endpoint:       "qux",
		CACertificates: []string{"fakecacert"},
		SkipTLSVerify:  true,
	}
	marshalled, err := cloud.MarshalCloud(in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(marshalled), tc.Equals, `
name: foo
type: bar
auth-types: [baz]
endpoint: qux
ca-certificates:
- fakecacert
skip-tls-verify: true
`[1:])
}

func (s *cloudSuite) TestUnmarshalCloud(c *tc.C) {
	in := []byte(`
name: foo
type: bar
auth-types: [baz]
endpoint: qux
ca-certificates: [fakecacert]
skip-tls-verify: true
`)
	out, err := cloud.UnmarshalCloud(in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, cloud.Cloud{
		Name:           "foo",
		Type:           "bar",
		AuthTypes:      []cloud.AuthType{"baz"},
		Endpoint:       "qux",
		CACertificates: []string{"fakecacert"},
		SkipTLSVerify:  true,
	})
}

func (s *cloudSuite) TestRegionByNameNoRegions(c *tc.C) {
	r, err := cloud.RegionByName([]cloud.Region{}, "star")
	c.Assert(r, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`region "star" not found (cloud has no regions)`))
}

func (s *cloudSuite) TestRegionByName(c *tc.C) {
	regions := []cloud.Region{
		{Name: "mars"},
		{Name: "earth"},
		{Name: "jupiter"},
	}

	r, err := cloud.RegionByName(regions, "mars")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(r, tc.Not(tc.IsNil))
	c.Assert(r, tc.DeepEquals, &cloud.Region{Name: "mars"})
}

func (s *cloudSuite) TestRegionByNameNotFound(c *tc.C) {
	regions := []cloud.Region{
		{Name: "mars"},
		{Name: "earth"},
		{Name: "jupiter"},
	}

	r, err := cloud.RegionByName(regions, "star")
	c.Assert(err, tc.ErrorMatches, regexp.QuoteMeta(`region "star" not found (expected one of ["earth" "jupiter" "mars"])`))
	c.Assert(err, tc.ErrorIs, errors.NotFound)
	c.Assert(r, tc.IsNil)
}
