// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package querylogger

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg.LogDir = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestStartConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	cfg.LogDir = c.MkDir()
	w, err := Manifold(cfg).Start(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	lw, ok := w.(*loggerWorker)
	c.Assert(ok, tc.IsTrue)
	c.Check(lw.clock, tc.Equals, clock.WallClock)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		LogDir: "log dir",
		Logger: s.logger,
	}
}
