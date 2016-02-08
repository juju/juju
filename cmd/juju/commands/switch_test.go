// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
	_ "github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type SwitchSimpleSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&SwitchSimpleSuite{})

func (s *SwitchSimpleSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)

	memstore := configstore.NewMem()
	s.PatchValue(&configstore.Default, func() (configstore.Storage, error) {
		return memstore, nil
	})

	memcache := jujuclienttesting.NewMem()
	s.PatchValue(&jujuclient.Default, func() (jujuclient.Cache, error) {
		return memcache, nil
	})
}

func (*SwitchSimpleSuite) TestNoDefault(c *gc.C) {
	_, err := testing.RunCommand(c, newSwitchCommand())
	c.Assert(err, gc.ErrorMatches, "no currently specified model")
}

func (s *SwitchSimpleSuite) TestCurrentModel(c *gc.C) {
	err := modelcmd.WriteCurrentModel("fubar")
	c.Assert(err, jc.ErrorIsNil)
	context, err := testing.RunCommand(c, newSwitchCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "fubar\n")
}

func (s *SwitchSimpleSuite) TestCurrentController(c *gc.C) {
	err := modelcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	context, err := testing.RunCommand(c, newSwitchCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "fubar (controller)\n")
}

func (*SwitchSimpleSuite) TestShowsJujuEnv(c *gc.C) {
	os.Setenv("JUJU_MODEL", "using-model")
	context, err := testing.RunCommand(c, newSwitchCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "using-model\n")
}

func (s *SwitchSimpleSuite) TestJujuEnvOverCurrentEnvironment(c *gc.C) {
	err := modelcmd.WriteCurrentModel("fubar")
	c.Assert(err, jc.ErrorIsNil)
	os.Setenv("JUJU_MODEL", "using-model")
	context, err := testing.RunCommand(c, newSwitchCommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "using-model\n")
}

func (s *SwitchSimpleSuite) TestSettingWritesFile(c *gc.C) {
	s.addTestEnv(c, "erewhemos-2")
	context, err := testing.RunCommand(c, newSwitchCommand(), "erewhemos-2")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "-> erewhemos-2\n")
	currentEnv, err := modelcmd.ReadCurrentModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currentEnv, gc.Equals, "erewhemos-2")
}

func (s *SwitchSimpleSuite) TestSettingWritesControllerFile(c *gc.C) {
	s.addTestController(c)
	context, err := testing.RunCommand(c, newSwitchCommand(), "a-controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(context), gc.Equals, "-> a-controller (controller)\n")
	currController, err := modelcmd.ReadCurrentController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currController, gc.Equals, "a-controller")
}

func (s *SwitchSimpleSuite) TestListWithController(c *gc.C) {
	s.addTestController(c)
	s.addTestEnv(c, "erewhemos")
	context, err := testing.RunCommand(c, newSwitchCommand(), "--list")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, `
a-controller (controller)
erewhemos
`[1:])
}

func (*SwitchSimpleSuite) TestSettingToUnknown(c *gc.C) {
	_, err := testing.RunCommand(c, newSwitchCommand(), "unknown")
	c.Assert(err, gc.ErrorMatches, `"unknown" is not a name of an existing defined model or controller`)
}

func (s *SwitchSimpleSuite) TestSettingWhenJujuEnvSet(c *gc.C) {
	s.addTestEnv(c, "erewhemos-2")
	os.Setenv("JUJU_MODEL", "using-model")
	_, err := testing.RunCommand(c, newSwitchCommand(), "erewhemos-2")
	c.Assert(err, gc.ErrorMatches, `cannot switch when JUJU_MODEL is overriding the model \(set to "using-model"\)`)
}

func (*SwitchSimpleSuite) TestListNoEnvironments(c *gc.C) {
	context, err := testing.RunCommand(c, newSwitchCommand(), "--list")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "")
}

func (s *SwitchSimpleSuite) TestListEnvironmentsWithConfigstore(c *gc.C) {
	s.addTestEnv(c, "erewhemos")
	s.addTestEnv(c, "erewhemos-2")
	context, err := testing.RunCommand(c, newSwitchCommand(), "--list")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "erewhemos\nerewhemos-2\n")
}

func (s *SwitchSimpleSuite) TestListEnvironmentsOSJujuEnvSet(c *gc.C) {
	s.addTestEnv(c, "erewhemos")
	s.addTestEnv(c, "erewhemos-2")
	os.Setenv("JUJU_MODEL", "using-model")
	context, err := testing.RunCommand(c, newSwitchCommand(), "--list")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stdout(context), gc.Equals, "erewhemos\nerewhemos-2\n")
}

func (s *SwitchSimpleSuite) TestListEnvironmentsAndChange(c *gc.C) {
	s.addTestEnv(c, "erewhemos-2")
	_, err := testing.RunCommand(c, newSwitchCommand(), "--list", "erewhemos-2")
	c.Assert(err, gc.ErrorMatches, "cannot switch and list at the same time")
}

func (*SwitchSimpleSuite) TestTooManyParams(c *gc.C) {
	_, err := testing.RunCommand(c, newSwitchCommand(), "foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: ."bar".`)
}

func (s *SwitchSimpleSuite) addTestController(c *gc.C) {
	// First set up a controller in the config store.
	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)
	info := store.CreateInfo("a-controller")

	endpoint := configstore.APIEndpoint{
		Addresses:  []string{"localhost"},
		CACert:     testing.CACert,
		ServerUUID: "server-uuid",
	}
	info.SetAPIEndpoint(endpoint)
	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)
	s.updateControllersFile(c, "a-controller", endpoint)
}

func (s *SwitchSimpleSuite) addTestEnv(c *gc.C, name string) {
	store, err := configstore.Default()
	c.Assert(err, jc.ErrorIsNil)
	info := store.CreateInfo(name)
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses: []string{"localhost"},
		CACert:    testing.CACert,
		ModelUUID: "model-uuid",
	})
	err = info.Write()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SwitchSimpleSuite) updateControllersFile(c *gc.C, name string, endpoint configstore.APIEndpoint) {
	cache, err := jujuclient.Default()
	c.Assert(err, jc.ErrorIsNil)
	err = cache.UpdateController(name,
		jujuclient.Controller{
			[]string{"test.host.name"},
			endpoint.ServerUUID,
			endpoint.Addresses,
			endpoint.CACert})
	c.Assert(err, jc.ErrorIsNil)
}
