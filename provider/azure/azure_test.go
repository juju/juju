// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdtesting "testing"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"launchpad.net/gwacl"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/environs/simplestreams"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

func TestAzureProvider(t *stdtesting.T) {
	gc.TestingT(t)
}

type providerSuite struct {
	testing.BaseSuite
	envtesting.ToolsFixture
	restoreTimeouts func()
}

var _ = gc.Suite(&providerSuite{})

var testRoundTripper = &jujutest.ProxyRoundTripper{}

func init() {
	// Prepare mock http transport for overriding metadata and images output in tests.
	testRoundTripper.RegisterForScheme("test")
}

func (s *providerSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	s.restoreTimeouts = envtesting.PatchAttemptStrategies()
	s.UploadArches = []string{arch.AMD64}
}

func (s *providerSuite) TearDownSuite(c *gc.C) {
	s.restoreTimeouts()
	s.BaseSuite.TearDownSuite(c)
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(&imagemetadata.DefaultBaseURL, "test:")
	s.PatchValue(&signedImageDataOnly, false)
	s.PatchValue(&getVirtualNetwork, func(*azureEnviron) (*gwacl.VirtualNetworkSite, error) {
		return &gwacl.VirtualNetworkSite{Name: "vnet", Location: "West US"}, nil
	})

	available := make(set.Strings)
	for _, rs := range gwacl.RoleSizes {
		available.Add(rs.Name)
	}
	s.PatchValue(&getAvailableRoleSizes, func(*azureEnviron) (set.Strings, error) {
		return available, nil
	})
}

func (s *providerSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *providerSuite) makeTestMetadata(c *gc.C, series, location string, im []*imagemetadata.ImageMetadata) {
	cloudSpec := simplestreams.CloudSpec{
		Region:   location,
		Endpoint: "https://management.core.windows.net/",
	}

	seriesVersion, err := version.SeriesVersion(series)
	c.Assert(err, jc.ErrorIsNil)
	for _, im := range im {
		im.Version = seriesVersion
		im.RegionName = cloudSpec.Region
		im.Endpoint = cloudSpec.Endpoint
	}

	index, products, err := imagemetadata.MarshalImageMetadataJSON(
		im, []simplestreams.CloudSpec{cloudSpec}, time.Now(),
	)
	c.Assert(err, jc.ErrorIsNil)
	files := map[string]string{
		"/streams/v1/index.json":                string(index),
		"/" + imagemetadata.ProductMetadataPath: string(products),
	}
	s.PatchValue(&testRoundTripper.Sub, jujutest.NewCannedRoundTripper(files, nil))
}
