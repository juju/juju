// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	clock "github.com/juju/clock"
	"github.com/juju/errors"
	coredb "github.com/juju/juju/core/db"
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

	cfg = s.getConfig()
	cfg.FileNotifyWatcher = ""
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.NewStream = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		DBAccessor:        "dbaccessor",
		FileNotifyWatcher: "filenotifywatcher",
		Clock:             s.clock,
		Logger:            s.logger,
		NewStream: func(coredb.TrackedDB, FileNotifier, clock.Clock, Logger) (DBStream, error) {
			return s.dbStream, nil
		},
	}
}
