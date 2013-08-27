// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"net/http"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
	coretesting "launchpad.net/juju-core/testing"
)

type ValidateSuite struct {
	coretesting.LoggingSuite
	home      *coretesting.FakeHome
	oldClient *http.Client
}

var _ = gc.Suite(&ValidateSuite{})

func (s *ValidateSuite) makeLocalMetadata(c *gc.C, id, region, series, endpoint string) error {
	im := imagemetadata.ImageMetadata{
		Id:   id,
		Arch: "amd64",
	}
	cloudSpec := simplestreams.CloudSpec{
		Region:   region,
		Endpoint: endpoint,
	}
	_, err := imagemetadata.MakeBoilerplate("", series, &im, &cloudSpec, false)
	if err != nil {
		return err
	}

	t := &http.Transport{}
	t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	s.oldClient = simplestreams.SetHttpClient(&http.Client{Transport: t})
	return nil
}

func (s *ValidateSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.home = coretesting.MakeEmptyFakeHome(c)
}

func (s *ValidateSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
	if s.oldClient != nil {
		simplestreams.SetHttpClient(s.oldClient)
	}
	s.LoggingSuite.TearDownTest(c)
}

func (s *ValidateSuite) TestMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1234", "region-2", "raring", "some-auth-url")
	metadataDir := config.JujuHomePath("")
	params := &simplestreams.MetadataLookupParams{
		Region:        "region-2",
		Series:        "raring",
		Architectures: []string{"amd64"},
		Endpoint:      "some-auth-url",
		BaseURLs:      []string{"file://" + metadataDir},
	}
	imageIds, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, gc.IsNil)
	c.Assert(imageIds, gc.DeepEquals, []string{"1234"})
}

func (s *ValidateSuite) TestNoMatch(c *gc.C) {
	s.makeLocalMetadata(c, "1234", "region-2", "raring", "some-auth-url")
	metadataDir := config.JujuHomePath("")
	params := &simplestreams.MetadataLookupParams{
		Region:        "region-2",
		Series:        "precise",
		Architectures: []string{"amd64"},
		Endpoint:      "some-auth-url",
		BaseURLs:      []string{"file://" + metadataDir},
	}
	_, err := imagemetadata.ValidateImageMetadata(params)
	c.Assert(err, gc.Not(gc.IsNil))
}
