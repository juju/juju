// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud/agent/machine"
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
		MachineStartup: func(api.Connection, machine.Logger) error {
			s.startCalled = true
			return nil
		},
		Logger: noOpLogger{},
	})
}

func (s *MachineStartupSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{
		"api-caller",
	})
}

func (s *MachineStartupSuite) TestStartSuccess(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller": new(mockAPIConn),
	})
	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "resource permanently unavailable")
	c.Check(s.startCalled, jc.IsTrue)
}

type mockAPIConn struct {
	api.Connection
}

type noOpLogger struct{}

func (noOpLogger) Warningf(string, ...interface{})  {}
func (noOpLogger) Criticalf(string, ...interface{}) {}
func (noOpLogger) Debugf(string, ...interface{})    {}
func (noOpLogger) Tracef(string, ...interface{})    {}
