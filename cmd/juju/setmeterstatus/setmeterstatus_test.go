// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package setmeterstatus_test

import (
	stdtesting "testing"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/setmeterstatus"
	"github.com/juju/juju/cmd/modelcmd"
	jujutesting "github.com/juju/juju/juju/testing"
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

func (s *SetMeterStatusSuite) TestUnit(c *gc.C) {
	client := MockSetMeterStatusClient{testing.Stub{}}
	s.PatchValue(setmeterstatus.NewClient, func(_ modelcmd.ModelCommandBase) (setmeterstatus.SetMeterStatusClient, error) {
		return &client, nil
	})
	_, err := coretesting.RunCommand(c, setmeterstatus.New(), "metered/0", "RED")
	c.Assert(err, jc.ErrorIsNil)
	client.CheckCall(c, 0, "SetMeterStatus", "unit-metered-0", "RED", "")
}

func (s *SetMeterStatusSuite) TestService(c *gc.C) {
	client := MockSetMeterStatusClient{testing.Stub{}}
	s.PatchValue(setmeterstatus.NewClient, func(_ modelcmd.ModelCommandBase) (setmeterstatus.SetMeterStatusClient, error) {
		return &client, nil
	})
	_, err := coretesting.RunCommand(c, setmeterstatus.New(), "metered", "RED")
	c.Assert(err, jc.ErrorIsNil)
	client.CheckCall(c, 0, "SetMeterStatus", "service-metered", "RED", "")
}

func (s *SetMeterStatusSuite) TestNotValidServiceOrUnit(c *gc.C) {
	client := MockSetMeterStatusClient{testing.Stub{}}
	s.PatchValue(setmeterstatus.NewClient, func(_ modelcmd.ModelCommandBase) (setmeterstatus.SetMeterStatusClient, error) {
		return &client, nil
	})
	_, err := coretesting.RunCommand(c, setmeterstatus.New(), "!!!!!!", "RED")
	c.Assert(err, gc.ErrorMatches, `"!!!!!!" is not a valid unit or service`)
}

type DebugMetricsCommandSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&DebugMetricsCommandSuite{})

func (s *DebugMetricsCommandSuite) TestDebugNoArgs(c *gc.C) {
	_, err := coretesting.RunCommand(c, setmeterstatus.New())
	c.Assert(err, gc.ErrorMatches, `you need to specify an entity \(service or unit\) and a status`)
}

func (s *DebugMetricsCommandSuite) TestUnits(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "mysql", URL: "local:quantal/mysql"})
	service := s.Factory.MakeService(c, &factory.ServiceParams{Charm: charm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: service, SetCharmURL: true})
	_, err := coretesting.RunCommand(c, setmeterstatus.New(), unit.Name(), "RED", "--info", "foobar")
	c.Assert(err, jc.ErrorIsNil)
	status, err := unit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code.String(), gc.Equals, "RED")
	c.Assert(status.Info, gc.Equals, "foobar")
}

func (s *DebugMetricsCommandSuite) TestService(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "mysql", URL: "local:quantal/mysql"})
	service := s.Factory.MakeService(c, &factory.ServiceParams{Charm: charm})
	unit0, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	unit1, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	_, err = coretesting.RunCommand(c, setmeterstatus.New(), "mysql", "RED", "--info", "foobar")
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
