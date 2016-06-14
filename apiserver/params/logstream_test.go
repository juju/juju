// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	"net/url"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

type LogStreamConfigSuite struct{}

var _ = gc.Suite(&LogStreamConfigSuite{})

func (s *LogStreamConfigSuite) TestEndpoint(c *gc.C) {
	var cfg params.LogStreamConfig

	ep := cfg.Endpoint()

	c.Check(ep, gc.Equals, "/logstream")
}

func (s *LogStreamConfigSuite) TestApplyFull(c *gc.C) {
	cfg := params.LogStreamConfig{
		AllModels: true,
		StartTime: time.Unix(12345, 10),
	}
	query := make(url.Values)

	cfg.Apply(query)

	c.Check(query, jc.DeepEquals, url.Values{
		"all":   []string{"true"},
		"start": []string{"12345"},
	})
}

func (s *LogStreamConfigSuite) TestApplyZeroValue(c *gc.C) {
	var cfg params.LogStreamConfig
	query := make(url.Values)

	cfg.Apply(query)

	c.Check(query, gc.HasLen, 0)
}

func (s *LogStreamConfigSuite) TestGetLogStreamConfigFull(c *gc.C) {
	query := url.Values{
		"all":   []string{"true"},
		"start": []string{"12345"},
	}

	cfg, err := params.GetLogStreamConfig(query)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cfg, jc.DeepEquals, params.LogStreamConfig{
		AllModels: true,
		StartTime: time.Unix(12345, 0),
	})
}

func (s *LogStreamConfigSuite) TestGetLogStreamConfigEmpty(c *gc.C) {
	var query url.Values

	cfg, err := params.GetLogStreamConfig(query)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cfg, jc.DeepEquals, params.LogStreamConfig{})
}

func (s *LogStreamConfigSuite) TestGetLogStreamConfigBadAllModels(c *gc.C) {
	query := url.Values{
		"all":   []string{"..."},
		"start": []string{"12345"},
	}

	_, err := params.GetLogStreamConfig(query)

	c.Check(err, gc.ErrorMatches, `all value "..." is not a valid boolean`)
}

func (s *LogStreamConfigSuite) TestGetLogStreamConfigStartTime(c *gc.C) {
	query := url.Values{
		"all":   []string{"true"},
		"start": []string{"..."},
	}

	_, err := params.GetLogStreamConfig(query)

	c.Check(err, gc.ErrorMatches, `start value "..." is not a valid unix timestamp`)
}
