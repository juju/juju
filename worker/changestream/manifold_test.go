// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"database/sql"

	clock "github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.Clock = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.DBAccessor = ""
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.NewStream = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		DBAccessor: "dbaccessor",
		Clock:      s.clock,
		Logger:     s.logger,
		NewStream: func(*sql.DB, clock.Clock, Logger) DBStream {
			return s.dbStream
		},
	}
}
