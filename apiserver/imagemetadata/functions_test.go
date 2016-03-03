// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/imagemetadata"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/testing"
)

type funcSuite struct {
	baseImageMetadataSuite

	env      environs.Environ
	expected cloudimagemetadata.Metadata
}

var _ = gc.Suite(&funcSuite{})

func (s *funcSuite) SetUpTest(c *gc.C) {
	s.baseImageMetadataSuite.SetUpTest(c)

	cfg, err := config.New(config.NoDefaults, mockConfig())
	c.Assert(err, jc.ErrorIsNil)
	s.env, err = environs.Prepare(
		envtesting.BootstrapContext(c), configstore.NewMem(),
		jujuclienttesting.NewMemStore(),
		"dummycontroller", environs.PrepareForBootstrapParams{Config: cfg},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.state = s.constructState(cfg)

	s.expected = cloudimagemetadata.Metadata{
		cloudimagemetadata.MetadataAttributes{
			Stream: "released",
			Source: "custom",
			Series: config.LatestLtsSeries(),
			Arch:   "amd64",
			Region: "dummy_region",
		},
		0,
		"",
	}
}

func (s *funcSuite) TestParseMetadataNoSource(c *gc.C) {
	m, err := imagemetadata.ParseMetadataFromParams(s.api, params.CloudImageMetadata{}, s.env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.DeepEquals, s.expected)
}

func (s *funcSuite) TestParseMetadataAnySource(c *gc.C) {
	s.expected.Source = "any"
	m, err := imagemetadata.ParseMetadataFromParams(s.api, params.CloudImageMetadata{Source: "any"}, s.env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.DeepEquals, s.expected)
}

func (s *funcSuite) TestParseMetadataAnyStream(c *gc.C) {
	stream := "happy stream"
	s.expected.Stream = stream

	m, err := imagemetadata.ParseMetadataFromParams(s.api, params.CloudImageMetadata{Stream: stream}, s.env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.DeepEquals, s.expected)
}

func (s *funcSuite) TestParseMetadataDefaultStream(c *gc.C) {
	m, err := imagemetadata.ParseMetadataFromParams(s.api, params.CloudImageMetadata{}, s.env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.DeepEquals, s.expected)
}

func (s *funcSuite) TestParseMetadataAnyRegion(c *gc.C) {
	region := "region"
	s.expected.Region = region

	m, err := imagemetadata.ParseMetadataFromParams(s.api, params.CloudImageMetadata{Region: region}, s.env)
	c.Assert(err, jc.ErrorIsNil)
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
