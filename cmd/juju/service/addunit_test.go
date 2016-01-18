// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

import (
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/juju/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/testing"
)

type AddUnitSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake *fakeServiceAddUnitAPI
}

type fakeServiceAddUnitAPI struct {
	envType   string
	service   string
	numUnits  int
	placement []*instance.Placement
	err       error
}

func (f *fakeServiceAddUnitAPI) Close() error {
	return nil
}

func (f *fakeServiceAddUnitAPI) ModelUUID() string {
	return "fake-uuid"
}

func (f *fakeServiceAddUnitAPI) AddUnits(service string, numUnits int, placement []*instance.Placement) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if service != f.service {
		return nil, errors.NotFoundf("service %q", service)
	}

	f.numUnits += numUnits
	f.placement = placement
	return nil, nil
}

func (f *fakeServiceAddUnitAPI) ModelGet() (map[string]interface{}, error) {
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"type": f.envType,
		"name": "dummy",
	})
	if err != nil {
		return nil, err
	}

	return cfg.AllAttrs(), nil
}

func (s *AddUnitSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.fake = &fakeServiceAddUnitAPI{service: "some-service-name", numUnits: 1, envType: "dummy"}
}

var _ = gc.Suite(&AddUnitSuite{})

var initAddUnitErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: []string{"some-service-name", "-n", "0"},
		err:  `--num-units must be a positive integer`,
	}, {
		args: []string{},
		err:  `no service specified`,
	}, {
		args: []string{"some-service-name", "--to", "1,#:foo"},
		err:  `invalid --to parameter "#:foo"`,
	},
}

func (s *AddUnitSuite) TestInitErrors(c *gc.C) {
	for i, t := range initAddUnitErrorTests {
		c.Logf("test %d", i)
		err := testing.InitCommand(service.NewAddUnitCommandForTest(s.fake), t.args)
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func (s *AddUnitSuite) runAddUnit(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, service.NewAddUnitCommandForTest(s.fake), args...)
	return err
}

func (s *AddUnitSuite) TestAddUnit(c *gc.C) {
	err := s.runAddUnit(c, "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)

	err = s.runAddUnit(c, "--num-units", "2", "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 4)
}

func (s *AddUnitSuite) TestAddUnitWithPlacement(c *gc.C) {
	err := s.runAddUnit(c, "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)

	err = s.runAddUnit(c, "--num-units", "2", "--to", "123,lxc:1,1/lxc/2,foo", "some-service-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 4)
	c.Assert(s.fake.placement, jc.DeepEquals, []*instance.Placement{
		{"#", "123"},
		{"lxc", "1"},
		{"#", "1/lxc/2"},
		{"fake-uuid", "foo"},
	})
}

func (s *AddUnitSuite) TestBlockAddUnit(c *gc.C) {
	// Block operation
	s.fake.err = common.OperationBlockedError("TestBlockAddUnit")
	s.runAddUnit(c, "some-service-name")

	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*TestBlockAddUnit.*")
}

func (s *AddUnitSuite) TestForceMachine(c *gc.C) {
	err := s.runAddUnit(c, "some-service-name", "--to", "3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)
	c.Assert(s.fake.placement[0].Directive, gc.Equals, "3")

	err = s.runAddUnit(c, "some-service-name", "--to", "23")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 3)
	c.Assert(s.fake.placement[0].Directive, gc.Equals, "23")
}

func (s *AddUnitSuite) TestForceMachineNewContainer(c *gc.C) {
	err := s.runAddUnit(c, "some-service-name", "--to", "lxc:1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)
	c.Assert(s.fake.placement[0].Directive, gc.Equals, "1")
	c.Assert(s.fake.placement[0].Scope, gc.Equals, "lxc")
}

func (s *AddUnitSuite) TestNameChecks(c *gc.C) {
	assertMachineOrNewContainer := func(s string, expect bool) {
		c.Logf("%s -> %v", s, expect)
		c.Assert(service.IsMachineOrNewContainer(s), gc.Equals, expect)
	}
	assertMachineOrNewContainer("0", true)
	assertMachineOrNewContainer("00", false)
	assertMachineOrNewContainer("1", true)
	assertMachineOrNewContainer("0/lxc/0", true)
	assertMachineOrNewContainer("lxc:0", true)
	assertMachineOrNewContainer("lxc:lxc:0", false)
	assertMachineOrNewContainer("kvm:0/lxc/1", true)
	assertMachineOrNewContainer("lxc:", false)
	assertMachineOrNewContainer(":lxc", false)
	assertMachineOrNewContainer("0/lxc/", false)
	assertMachineOrNewContainer("0/lxc", false)
	assertMachineOrNewContainer("kvm:0/lxc", false)
	assertMachineOrNewContainer("0/lxc/01", false)
	assertMachineOrNewContainer("0/lxc/10", true)
	assertMachineOrNewContainer("0/kvm/4", true)
}
