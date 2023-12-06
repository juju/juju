// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type deployerSuite struct {
	baseSuite
}

var _ = gc.Suite(&deployerSuite{})

func (s *deployerSuite) TestValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig()
	err := cfg.Validate()
	c.Assert(err, gc.IsNil)

	cfg = s.newConfig()
	cfg.DataDir = ""
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig()
	cfg.State = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig()
	cfg.ControllerConfig = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig()
	cfg.NewCharmRepo = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig()
	cfg.NewCharmDownloader = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig()
	cfg.CharmhubHTTPClient = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig()
	cfg.LoggerFactory = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}
