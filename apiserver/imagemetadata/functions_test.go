// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
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

type funcMetadataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&funcMetadataSuite{})

func (s *funcMetadataSuite) TestProcessErrorsNil(c *gc.C) {
	s.assertProcessErrorsNone(c, nil)
}

func (s *funcMetadataSuite) TestProcessErrorsEmpty(c *gc.C) {
	s.assertProcessErrorsNone(c, []params.ErrorResult{})
}

func (s *funcMetadataSuite) TestProcessErrorsNilError(c *gc.C) {
	s.assertProcessErrorsNone(c, []params.ErrorResult{{Error: nil}})
}

func (s *funcMetadataSuite) TestProcessErrorsEmptyMessageError(c *gc.C) {
	s.assertProcessErrorsNone(c, []params.ErrorResult{{Error: &params.Error{Message: ""}}})
}

func (s *funcMetadataSuite) TestProcessErrorsFullError(c *gc.C) {
	msg := "my bad"

	errs := []params.ErrorResult{{Error: &params.Error{Message: msg}}}

	expected := fmt.Sprintf(`
saving some image metadata:
%v`[1:], msg)

	s.assertProcessErrors(c, errs, expected)
}

func (s *funcMetadataSuite) TestProcessErrorsMany(c *gc.C) {
	msg1 := "my bad"
	msg2 := "my good"

	errs := []params.ErrorResult{
		{Error: &params.Error{Message: msg1}},
		{Error: &params.Error{Message: ""}},
		{Error: nil},
		{Error: &params.Error{Message: msg2}},
	}

	expected := fmt.Sprintf(`
saving some image metadata:
%v
%v`[1:], msg1, msg2)

	s.assertProcessErrors(c, errs, expected)
}

var process = imagemetadata.ProcessErrors

func (s *funcMetadataSuite) assertProcessErrorsNone(c *gc.C, errs []params.ErrorResult) {
	c.Assert(process(errs), jc.ErrorIsNil)
}

func (s *funcMetadataSuite) assertProcessErrors(c *gc.C, errs []params.ErrorResult, expected string) {
	c.Assert(process(errs), gc.ErrorMatches, expected)
}
