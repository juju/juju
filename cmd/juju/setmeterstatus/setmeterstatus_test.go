// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package setmeterstatus_test

import (
	stdtesting "testing"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/setmeterstatus"
	"github.com/juju/juju/cmd/modelcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type MockSetMeterStatusClient struct {
	testing.Stub
}

func (m *MockSetMeterStatusClient) SetMeterStatus(tag, status, info string) error {
	m.Stub.MethodCall(m, "SetMeterStatus", tag, status, info)
	return m.NextErr()
}
func (m *MockSetMeterStatusClient) Close() error {
	m.Stub.MethodCall(m, "Close")
	return nil
}

type SetMeterStatusSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&SetMeterStatusSuite{})

func setMeterStatusCommand() cmd.Command {
	return setmeterstatus.NewCommandForTest(jujuclienttesting.MinimalStore())
}

func (s *SetMeterStatusSuite) TestUnit(c *gc.C) {
	client := MockSetMeterStatusClient{testing.Stub{}}
	s.PatchValue(setmeterstatus.NewClient, func(_ modelcmd.ModelCommandBase) (setmeterstatus.SetMeterStatusClient, error) {
		return &client, nil
	})
	_, err := cmdtesting.RunCommand(c, setMeterStatusCommand(), "metered/0", "RED")
	c.Assert(err, jc.ErrorIsNil)
	client.CheckCall(c, 0, "SetMeterStatus", "unit-metered-0", "RED", "")
}

func (s *SetMeterStatusSuite) TestApplication(c *gc.C) {
	client := MockSetMeterStatusClient{testing.Stub{}}
	s.PatchValue(setmeterstatus.NewClient, func(_ modelcmd.ModelCommandBase) (setmeterstatus.SetMeterStatusClient, error) {
		return &client, nil
	})
	_, err := cmdtesting.RunCommand(c, setMeterStatusCommand(), "metered", "RED")
	c.Assert(err, jc.ErrorIsNil)
	client.CheckCall(c, 0, "SetMeterStatus", "application-metered", "RED", "")
}

func (s *SetMeterStatusSuite) TestNotValidApplicationOrUnit(c *gc.C) {
	client := MockSetMeterStatusClient{testing.Stub{}}
	s.PatchValue(setmeterstatus.NewClient, func(_ modelcmd.ModelCommandBase) (setmeterstatus.SetMeterStatusClient, error) {
		return &client, nil
	})
	_, err := cmdtesting.RunCommand(c, setMeterStatusCommand(), "!!!!!!", "RED")
	c.Assert(err, gc.ErrorMatches, `"!!!!!!" is not a valid unit or application`)
}

type DebugMetricsCommandSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&DebugMetricsCommandSuite{})

func (s *DebugMetricsCommandSuite) TestDebugNoArgs(c *gc.C) {
	cmd := setmeterstatus.NewCommandForTest(s.ControllerStore)
	_, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, `you need to specify an entity \(application or unit\) and a status`)
}

func (s *DebugMetricsCommandSuite) TestUnits(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "mysql", URL: "local:quantal/mysql-1"})
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: charm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: app, SetCharmURL: true})
	cmd := setmeterstatus.NewCommandForTest(s.ControllerStore)
	_, err := cmdtesting.RunCommand(c, cmd, unit.Name(), "RED", "--info", "foobar")
	c.Assert(err, jc.ErrorIsNil)
	status, err := unit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code.String(), gc.Equals, "RED")
	c.Assert(status.Info, gc.Equals, "foobar")
}

func (s *DebugMetricsCommandSuite) TestApplication(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "mysql", URL: "local:quantal/mysql-1"})
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: charm})
	unit0, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	unit1, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	cmd := setmeterstatus.NewCommandForTest(s.ControllerStore)
	_, err = cmdtesting.RunCommand(c, cmd, "mysql", "RED", "--info", "foobar")
	c.Assert(err, jc.ErrorIsNil)
	status, err := unit0.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code.String(), gc.Equals, "RED")
	c.Assert(status.Info, gc.Equals, "foobar")

	status, err = unit1.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code.String(), gc.Equals, "RED")
	c.Assert(status.Info, gc.Equals, "foobar")
}
