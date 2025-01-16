// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	dt "github.com/juju/worker/v3/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	dbtesting "github.com/juju/juju/database/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/sshserver"
	"github.com/juju/testing"
)

type manifoldSuite struct {
	dbtesting.ControllerSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestConfigValidate(c *gc.C) {
	cfg := sshserver.ManifoldConfig{
		StateName: "state",
	}

	c.Assert(cfg.Validate(), gc.IsNil)

	cfg = sshserver.ManifoldConfig{}
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)
}

func (s *manifoldSuite) TestManifoldStart(c *gc.C) {
	manifold := sshserver.Manifold(sshserver.ManifoldConfig{
		StateName: "state",
		NewWorker: func(sp *state.StatePool, jumpHostKey string) (worker.Worker, error) { return nil, nil },
	})

	tracker := &stubStateTracker{}
	worker, err := manifold.Start(
		dt.StubContext(nil, map[string]interface{}{
			"state": tracker,
		}),
	)
	c.Assert(worker, gc.NotNil)
	c.Assert(err, gc.IsNil)

	tracker.CheckCallNames(c, "Use")
}

type stubStateTracker struct {
	testing.Stub
	pool state.StatePool
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return &s.pool, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	return s.NextErr()
}

func (s *stubStateTracker) Report() map[string]interface{} {
	s.MethodCall(s, "Report")
	return nil
}
