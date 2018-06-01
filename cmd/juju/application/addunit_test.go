// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"strings"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiapplication "github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/application"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type AddUnitSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	fake *fakeApplicationAddUnitAPI
}

type fakeApplicationAddUnitAPI struct {
	envType        string
	application    string
	numUnits       int
	placement      []*instance.Placement
	attachStorage  []string
	bestAPIVersion int
	err            error
}

func (f *fakeApplicationAddUnitAPI) BestAPIVersion() int {
	return f.bestAPIVersion
}

func (f *fakeApplicationAddUnitAPI) Close() error {
	return nil
}

func (f *fakeApplicationAddUnitAPI) ModelUUID() string {
	return "fake-uuid"
}

func (f *fakeApplicationAddUnitAPI) AddUnits(args apiapplication.AddUnitsParams) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if args.ApplicationName != f.application {
		return nil, errors.NotFoundf("application %q", args.ApplicationName)
	}

	f.numUnits += args.NumUnits
	f.placement = args.Placement
	f.attachStorage = args.AttachStorage
	return nil, nil
}

func (f *fakeApplicationAddUnitAPI) ModelGet() (map[string]interface{}, error) {
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
	s.fake = &fakeApplicationAddUnitAPI{
		application:    "some-application-name",
		numUnits:       1,
		envType:        "dummy",
		bestAPIVersion: 5,
	}
}

var _ = gc.Suite(&AddUnitSuite{})

var initAddUnitErrorTests = []struct {
	args []string
	err  string
}{
	{
		args: []string{"some-application-name", "-n", "0"},
		err:  `--num-units must be a positive integer`,
	}, {
		args: []string{},
		err:  `no application specified`,
	}, {
		args: []string{"some-application-name", "--to", "1,#:foo"},
		err:  `invalid --to parameter "#:foo"`,
	}, {
		args: []string{"some-application-name", "--attach-storage", "foo/0", "-n", "2"},
		err:  `--attach-storage cannot be used with -n`,
	},
}

func (s *AddUnitSuite) TestInitErrors(c *gc.C) {
	for i, t := range initAddUnitErrorTests {
		c.Logf("test %d", i)
		err := cmdtesting.InitCommand(application.NewAddUnitCommandForTest(s.fake, jujuclienttesting.MinimalStore()), t.args)
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

func (s *AddUnitSuite) runAddUnit(c *gc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, application.NewAddUnitCommandForTest(s.fake, jujuclienttesting.MinimalStore()), args...)
	return err
}

func (s *AddUnitSuite) TestAddUnit(c *gc.C) {
	err := s.runAddUnit(c, "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)

	err = s.runAddUnit(c, "--num-units", "2", "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 4)
}

func (s *AddUnitSuite) TestAddUnitWithPlacement(c *gc.C) {
	err := s.runAddUnit(c, "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)

	err = s.runAddUnit(c, "--num-units", "2", "--to", "123,lxd:1,1/lxd/2,foo", "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 4)
	c.Assert(s.fake.placement, jc.DeepEquals, []*instance.Placement{
		{"#", "123"},
		{"lxd", "1"},
		{"#", "1/lxd/2"},
		{"fake-uuid", "foo"},
	})
}

func (s *AddUnitSuite) TestAddUnitAttachStorage(c *gc.C) {
	err := s.runAddUnit(c, "some-application-name")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)
	c.Assert(s.fake.attachStorage, gc.HasLen, 0)

	err = s.runAddUnit(c, "some-application-name", "--attach-storage", "foo/0,bar/1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 3)
	c.Assert(s.fake.attachStorage, jc.DeepEquals, []string{"foo/0", "bar/1"})
}

func (s *AddUnitSuite) TestAddUnitAttachStorageNotSupported(c *gc.C) {
	s.fake.bestAPIVersion = 4 // v4 does not support attach-storage
	err := s.runAddUnit(c, "some-application-name", "--attach-storage", "foo/0")
	c.Assert(err, gc.ErrorMatches, "this juju controller does not support --attach-storage")
}

func (s *AddUnitSuite) TestBlockAddUnit(c *gc.C) {
	// Block operation
	s.fake.err = common.OperationBlockedError("TestBlockAddUnit")
	s.runAddUnit(c, "some-application-name")

	// msg is logged
	stripped := strings.Replace(c.GetTestLog(), "\n", "", -1)
	c.Check(stripped, gc.Matches, ".*TestBlockAddUnit.*")
}

func (s *AddUnitSuite) TestUnauthorizedMentionsJujuGrant(c *gc.C) {
	s.fake.err = &params.Error{
		Message: "permission denied",
		Code:    params.CodeUnauthorized,
	}
	ctx, _ := cmdtesting.RunCommand(c, application.NewAddUnitCommandForTest(
		s.fake, jujuclienttesting.MinimalStore()), "some-application-name")
	errString := strings.Replace(cmdtesting.Stderr(ctx), "\n", " ", -1)
	c.Assert(errString, gc.Matches, `.*juju grant.*`)
}

func (s *AddUnitSuite) TestForceMachine(c *gc.C) {
	err := s.runAddUnit(c, "some-application-name", "--to", "3")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)
	c.Assert(s.fake.placement[0].Directive, gc.Equals, "3")

	err = s.runAddUnit(c, "some-application-name", "--to", "23")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 3)
	c.Assert(s.fake.placement[0].Directive, gc.Equals, "23")
}

func (s *AddUnitSuite) TestForceMachineNewContainer(c *gc.C) {
	err := s.runAddUnit(c, "some-application-name", "--to", "lxd:1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.fake.numUnits, gc.Equals, 2)
	c.Assert(s.fake.placement[0].Directive, gc.Equals, "1")
	c.Assert(s.fake.placement[0].Scope, gc.Equals, "lxd")
}

func (s *AddUnitSuite) TestNameChecks(c *gc.C) {
	assertMachineOrNewContainer := func(s string, expect bool) {
		c.Logf("%s -> %v", s, expect)
		c.Assert(application.IsMachineOrNewContainer(s), gc.Equals, expect)
	}
	assertMachineOrNewContainer("0", true)
	assertMachineOrNewContainer("00", false)
	assertMachineOrNewContainer("1", true)
	assertMachineOrNewContainer("0/lxd/0", true)
	assertMachineOrNewContainer("lxd:0", true)
	assertMachineOrNewContainer("lxd:lxd:0", false)
	assertMachineOrNewContainer("kvm:0/lxd/1", true)
	assertMachineOrNewContainer("lxd:", false)
	assertMachineOrNewContainer(":lxd", false)
	assertMachineOrNewContainer("0/lxd/", false)
	assertMachineOrNewContainer("0/lxd", false)
	assertMachineOrNewContainer("kvm:0/lxd", false)
	assertMachineOrNewContainer("0/lxd/01", false)
	assertMachineOrNewContainer("0/lxd/10", true)
	assertMachineOrNewContainer("0/kvm/4", true)
}
