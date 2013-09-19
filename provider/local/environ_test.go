// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/local"
	jc "launchpad.net/juju-core/testing/checkers"
)

type environSuite struct {
	baseProviderSuite
}

var _ = gc.Suite(&environSuite{})

func (*environSuite) TestOpenFailsWithProtectedDirectories(c *gc.C) {
	testConfig := minimalConfig(c)
	testConfig, err := testConfig.Apply(map[string]interface{}{
		"root-dir": "/usr/lib/juju",
	})
	c.Assert(err, gc.IsNil)

	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.ErrorMatches, "mkdir .* permission denied")
	c.Assert(environ, gc.IsNil)
}

func (s *environSuite) TestNameAndStorage(c *gc.C) {
	testConfig := minimalConfig(c)
	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.IsNil)
	c.Assert(environ.Name(), gc.Equals, "test")
	c.Assert(environ.Storage(), gc.NotNil)
	c.Assert(environ.PublicStorage(), gc.NotNil)
}

func (s *environSuite) TestPrecheck(c *gc.C) {
	testConfig := minimalConfig(c)
	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.IsNil)
	var cons constraints.Value
	prechecker, ok := environ.(environs.Prechecker)
	c.Assert(ok, jc.IsTrue)

	err = prechecker.PrecheckCreateMachine("precise", cons)
	c.Check(err, gc.IsNil)
	container := instance.LXC

	var inst instance.Instance
	cons.Container = &container
	err = prechecker.PrecheckCreateContainer("precise", cons, inst)
	c.Check(err, gc.ErrorMatches, "local provider does not support nested containers")
}

type localJujuTestSuite struct {
	baseProviderSuite
	jujutest.Tests
	restoreRootCheck   func()
	oldUpstartLocation string
	oldPath            string
	testPath           string
	dbServiceName      string
}

func (s *localJujuTestSuite) SetUpTest(c *gc.C) {
	s.baseProviderSuite.SetUpTest(c)
	// Construct the directories first.
	err := local.CreateDirs(c, minimalConfig(c))
	c.Assert(err, gc.IsNil)
	s.oldUpstartLocation = local.SetUpstartScriptLocation(c.MkDir())
	s.oldPath = os.Getenv("PATH")
	s.testPath = c.MkDir()
	os.Setenv("PATH", s.testPath+":"+s.oldPath)

	// Add in an admin secret
	s.Tests.TestConfig["admin-secret"] = "sekrit"
	s.restoreRootCheck = local.SetRootCheckFunction(func() bool { return true })
	s.Tests.SetUpTest(c)
	s.dbServiceName = "juju-db-" + local.ConfigNamespace(s.Env.Config())
}

func (s *localJujuTestSuite) TearDownTest(c *gc.C) {
	s.Tests.TearDownTest(c)
	os.Setenv("PATH", s.oldPath)
	s.restoreRootCheck()
	local.SetUpstartScriptLocation(s.oldUpstartLocation)
	s.baseProviderSuite.TearDownTest(c)
}

func (s *localJujuTestSuite) MakeTool(c *gc.C, name, script string) {
	path := filepath.Join(s.testPath, name)
	script = "#!/bin/bash\n" + script
	err := ioutil.WriteFile(path, []byte(script), 0755)
	c.Assert(err, gc.IsNil)
}

func (s *localJujuTestSuite) StoppedStatus(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service stop/waiting"`)
}

func (s *localJujuTestSuite) RunningStatus(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service start/running, process 123"`)
}

var _ = gc.Suite(&localJujuTestSuite{
	Tests: jujutest.Tests{
		TestConfig: minimalConfigValues(),
	},
})

func (s *localJujuTestSuite) TestBootstrap(c *gc.C) {
	c.Skip("Cannot test bootstrap at this stage.")
}

func (s *localJujuTestSuite) TestStartStop(c *gc.C) {
	c.Skip("StartInstance not implemented yet.")
}
