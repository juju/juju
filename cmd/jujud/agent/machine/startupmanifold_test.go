// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud/agent/machine"
	corelogger "github.com/juju/juju/core/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
)

type MachineStartupSuite struct {
	testing.BaseSuite
	manifold    dependency.Manifold
	startCalled bool
}

func TestMachineStartupSuite(t *stdtesting.T) { tc.Run(t, &MachineStartupSuite{}) }
func (s *MachineStartupSuite) SetUpTest(c *tc.C) {
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

func (s *MachineStartupSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.SameContents, []string{
		"api-caller",
	})
}

func (s *MachineStartupSuite) TestStartSuccess(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller": new(mockAPIConn),
	})
	worker, err := s.manifold.Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "resource permanently unavailable")
	c.Check(s.startCalled, tc.IsTrue)
}

type mockAPIConn struct {
	api.Connection
}
