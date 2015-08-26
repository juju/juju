// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/imagemetadata"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/testing"
)

type funcSuite struct {
	testing.BaseSuite

	expected cloudimagemetadata.Metadata
}

func (s *funcSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.expected = cloudimagemetadata.Metadata{
		cloudimagemetadata.MetadataAttributes{
			Stream: "released",
			Source: cloudimagemetadata.Custom,
		}, ""}
}

var _ = gc.Suite(&funcSuite{})

func (s *funcSuite) TestParseMetadataEmpty(c *gc.C) {
	m := imagemetadata.ParseMetadataFromParams(params.CloudImageMetadata{})
	c.Assert(m, gc.DeepEquals, s.expected)
}

func (s *funcSuite) TestParseMetadataPublic(c *gc.C) {
	s.expected.Source = cloudimagemetadata.Public

	m := imagemetadata.ParseMetadataFromParams(params.CloudImageMetadata{Source: "public"})
	c.Assert(m, gc.DeepEquals, s.expected)

	m = imagemetadata.ParseMetadataFromParams(params.CloudImageMetadata{Source: "PubLic"})
	c.Assert(m, gc.DeepEquals, s.expected)
}

func (s *funcSuite) TestParseMetadataWithStream(c *gc.C) {
	stream := "happy stream"
	s.expected.Stream = stream

	m := imagemetadata.ParseMetadataFromParams(params.CloudImageMetadata{Stream: stream})
	c.Assert(m, gc.DeepEquals, s.expected)
}
