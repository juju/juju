// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud-controller/agent/machine"
	corelogger "github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/testing"
)

type MachineStartupSuite struct {
	testing.BaseSuite
	manifold    dependency.Manifold
	startCalled bool
}

var _ = gc.Suite(&MachineStartupSuite{})

func (s *MachineStartupSuite) SetUpTest(c *gc.C) {
	s.startCalled = false
	s.manifold = machine.MachineStartupManifold(machine.MachineStartupConfig{
		APICallerName: "api-caller",
		MachineStartup: func(context.Context, api.Connection, corelogger.Logger) error {
			s.startCalled = true
			return nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	})
}

func (s *MachineStartupSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{
		"api-caller",
	})
}

func (s *MachineStartupSuite) TestStartSuccess(c *gc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller": new(mockAPIConn),
	})
	worker, err := s.manifold.Start(context.Background(), getter)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "resource permanently unavailable")
	c.Check(s.startCalled, jc.IsTrue)
}

type mockAPIConn struct {
	api.Connection
}
