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
		},
		"",
	}
}

var _ = gc.Suite(&funcSuite{})

func (s *funcSuite) TestParseMetadataSourcePanic(c *gc.C) {
	m := func() { imagemetadata.ParseMetadataFromParams(params.CloudImageMetadata{}) }
	c.Assert(m, gc.PanicMatches, `unknown cloud image metadata source ""`)
}

func (s *funcSuite) TestParseMetadataCustom(c *gc.C) {
	m := imagemetadata.ParseMetadataFromParams(params.CloudImageMetadata{Source: "custom"})
	c.Assert(m, gc.DeepEquals, s.expected)

	m = imagemetadata.ParseMetadataFromParams(params.CloudImageMetadata{Source: "CusTOM"})
	c.Assert(m, gc.DeepEquals, s.expected)
}

func (s *funcSuite) TestParseMetadataPublic(c *gc.C) {
	s.expected.Source = cloudimagemetadata.Public

	m := imagemetadata.ParseMetadataFromParams(params.CloudImageMetadata{Source: "public"})
	c.Assert(m, gc.DeepEquals, s.expected)

	m = imagemetadata.ParseMetadataFromParams(params.CloudImageMetadata{Source: "PubLic"})
	c.Assert(m, gc.DeepEquals, s.expected)
}

func (s *funcSuite) TestParseMetadataAnyStream(c *gc.C) {
	stream := "happy stream"
	s.expected.Stream = stream

	m := imagemetadata.ParseMetadataFromParams(params.CloudImageMetadata{
		Source: "custom",
		Stream: stream,
	})
	c.Assert(m, gc.DeepEquals, s.expected)
}

func (s *funcSuite) TestParseMetadataDefaultStream(c *gc.C) {
	m := imagemetadata.ParseMetadataFromParams(params.CloudImageMetadata{
		Source: "custom",
	})
	c.Assert(m, gc.DeepEquals, s.expected)
}
