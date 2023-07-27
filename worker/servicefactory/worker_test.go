// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type workerSuite struct {
	baseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.DBGetter = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.NewServiceFactoryGetter = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.NewControllerServiceFactory = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	cfg = s.getConfig()
	cfg.NewModelServiceFactory = nil
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)
}

func (s *workerSuite) getConfig() Config {
	return Config{
		DBGetter:  s.dbGetter,
		DBDeleter: s.dbDeleter,
		Logger:    s.logger,
		NewServiceFactoryGetter: func(ControllerServiceFactory, changestream.WatchableDBGetter, Logger, ModelServiceFactoryFn) ServiceFactoryGetter {
			return nil
		},
		NewControllerServiceFactory: func(changestream.WatchableDBGetter, coredatabase.DBDeleter, Logger) ControllerServiceFactory {
			return nil
		},
		NewModelServiceFactory: func(changestream.WatchableDBGetter, string, Logger) ModelServiceFactory {
			return nil
		},
	}
}
