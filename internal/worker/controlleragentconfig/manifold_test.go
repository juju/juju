// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"path/filepath"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/dependency"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/internal/worker/gate"
)

type manifoldSuite struct {
	baseSuite

	socketDir string
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	s.socketDir = c.MkDir()
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.ControllerID = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewSocketListener = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.SocketName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ReadyUnlocker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		ControllerID:      "99",
		Logger:            s.logger,
		NewSocketListener: NewSocketListener,
		SocketName:        filepath.Join(s.socketDir, "test.socket"),
		ReadyUnlocker:     gate.NewLock(),
	}
}

func (s *manifoldSuite) newContext() dependency.Getter {
	return dependencytesting.StubGetter(map[string]any{})
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, tc.HasLen, 0)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := Manifold(s.getConfig()).Start(c.Context(), s.newContext())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestStartConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := Manifold(s.getConfig()).Start(c.Context(), s.newContext())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	cw, ok := w.(*configWorker)
	c.Assert(ok, tc.IsTrue)
	c.Check(cw.cfg.ControllerID, tc.Equals, "99")
}

// TestStartUnlocksReadyGate verifies that a successful Start call unlocks
// the ReadyUnlocker, signalling to dependents that the socket is available.
func (s *manifoldSuite) TestStartUnlocksReadyGate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	lock := gate.NewLock()
	cfg := s.getConfig()
	cfg.ReadyUnlocker = lock

	c.Assert(lock.IsUnlocked(), tc.IsFalse)

	w, err := Manifold(cfg).Start(c.Context(), s.newContext())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	c.Check(lock.IsUnlocked(), tc.IsTrue)
}

func (s *manifoldSuite) TestOutput(c *tc.C) {
	defer s.setupMocks(c).Finish()

	man := Manifold(s.getConfig())
	w, err := man.Start(c.Context(), s.newContext())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	var watcher ConfigWatcher
	c.Assert(man.Output(w, &watcher), tc.ErrorIsNil)
	c.Assert(watcher, tc.NotNil)
}
