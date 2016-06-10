// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"net/url"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

type StreamFormatSuite struct{}

var _ = gc.Suite(&StreamFormatSuite{})

func (s *StreamFormatSuite) TestApplyFormatSupported(c *gc.C) {
	var cfg params.StreamConfig
	for format, expected := range map[params.StreamFormat]string{
		params.StreamFormatRaw:  "",
		params.StreamFormatJSON: "json",
	} {
		c.Logf("trying %q", format)
		cfg.Format = format
		query := make(url.Values)

		cfg.Apply(query)

		value := query.Get("format")
		c.Check(value, gc.Equals, expected)
	}
}

func (s *StreamFormatSuite) TestApplyZeroValue(c *gc.C) {
	var cfg params.StreamConfig
	query := make(url.Values)

	cfg.Apply(query)

	c.Check(query, gc.HasLen, 0)
}

func (s *StreamFormatSuite) TestGetStreamConfigFormatSupported(c *gc.C) {
	for value, expected := range map[string]params.StreamFormat{
		"":     params.StreamFormatRaw,
		"json": params.StreamFormatJSON,
	} {
		c.Logf("trying %q", value)
		query := make(url.Values)
		query.Set("format", value)

		cfg, err := params.GetStreamConfig(query)
		c.Assert(err, jc.ErrorIsNil)

		c.Check(cfg, jc.DeepEquals, params.StreamConfig{
			Format: expected,
		})
	}
}

func (s *StreamFormatSuite) TestGetStreamConfigEmpty(c *gc.C) {
	query := make(url.Values)

	cfg, err := params.GetStreamConfig(query)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cfg, jc.DeepEquals, params.StreamConfig{
		Format: params.StreamFormatRaw,
	})
}

func (s *StreamFormatSuite) TestGetStreamConfigFormatUnsupported(c *gc.C) {
	for _, value := range []string{
		"...",
		"spam",
		"JSON",
		"jsonX",
		"jso",
	} {
		c.Logf("trying %q", value)
		query := make(url.Values)
		query.Set("format", value)

		_, err := params.GetStreamConfig(query)

		c.Check(err, gc.ErrorMatches, `unsupported stream format "`+value+`"`)
	}
}
