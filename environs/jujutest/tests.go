// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

// Tests is a gocheck suite containing tests verifying juju functionality
// against the environment with the given configuration. The
// tests are not designed to be run against a live server - the Environ
// is opened once for each test, and some potentially expensive operations
// may be executed.
type Tests struct {
	TestConfig coretesting.Attrs
	envtesting.ToolsFixture

	// ConfigStore holds the configuration storage
	// used when preparing the environment.
	// This is initialized by SetUpTest.
	ConfigStore configstore.Storage
}

// Open opens an instance of the testing environment.
func (t *Tests) Open(c *gc.C) environs.Environ {
	info, err := t.ConfigStore.ReadInfo(t.TestConfig["name"].(string))
	c.Assert(err, gc.IsNil)
	cfg, err := config.New(config.NoDefaults, info.BootstrapConfig())
	c.Assert(err, gc.IsNil)
	e, err := environs.New(cfg)
	c.Assert(err, gc.IsNil, gc.Commentf("opening environ %#v", cfg.AllAttrs()))
	c.Assert(e, gc.NotNil)
	return e
}

// Prepare prepares an instance of the testing environment.
func (t *Tests) Prepare(c *gc.C) environs.Environ {
	cfg, err := config.New(config.NoDefaults, t.TestConfig)
	c.Assert(err, gc.IsNil)
	e, err := environs.Prepare(cfg, coretesting.Context(c), t.ConfigStore)
	c.Assert(err, gc.IsNil, gc.Commentf("preparing environ %#v", t.TestConfig))
	c.Assert(e, gc.NotNil)
	return e
}

func (t *Tests) SetUpTest(c *gc.C) {
	storageDir := c.MkDir()
	t.DefaultBaseURL = "file://" + storageDir + "/tools"
	t.ToolsFixture.SetUpTest(c)
	t.UploadFakeToolsToDirectory(c, storageDir)
	t.ConfigStore = configstore.NewMem()
}

func (t *Tests) TearDownTest(c *gc.C) {
	t.ToolsFixture.TearDownTest(c)
}

func (t *Tests) TestStartStop(c *gc.C) {
	e := t.Prepare(c)
	cfg, err := e.Config().Apply(map[string]interface{}{
		"agent-version": version.Current.Number.String(),
	})
	c.Assert(err, gc.IsNil)
	err = e.SetConfig(cfg)
	c.Assert(err, gc.IsNil)

	insts, err := e.Instances(nil)
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 0)

	inst0, hc := testing.AssertStartInstance(c, e, "0")
	c.Assert(inst0, gc.NotNil)
	id0 := inst0.Id()
	// Sanity check for hardware characteristics.
	c.Assert(hc.Arch, gc.NotNil)
	c.Assert(hc.Mem, gc.NotNil)
	c.Assert(hc.CpuCores, gc.NotNil)

	inst1, _ := testing.AssertStartInstance(c, e, "1")
	c.Assert(inst1, gc.NotNil)
	id1 := inst1.Id()

	insts, err = e.Instances([]instance.Id{id0, id1})
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 2)
	c.Assert(insts[0].Id(), gc.Equals, id0)
	c.Assert(insts[1].Id(), gc.Equals, id1)

	// order of results is not specified
	insts, err = e.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 2)
	c.Assert(insts[0].Id(), gc.Not(gc.Equals), insts[1].Id())

	err = e.StopInstances(inst0.Id())
	c.Assert(err, gc.IsNil)

	insts, err = e.Instances([]instance.Id{id0, id1})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(insts[0], gc.IsNil)
	c.Assert(insts[1].Id(), gc.Equals, id1)

	insts, err = e.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(insts[0].Id(), gc.Equals, id1)
}

func (t *Tests) TestBootstrap(c *gc.C) {
	e := t.Prepare(c)
	err := bootstrap.EnsureNotBootstrapped(e)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(coretesting.Context(c), e, bootstrap.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	stateServerInstances, err := e.StateServerInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(stateServerInstances, gc.Not(gc.HasLen), 0)

	err = bootstrap.EnsureNotBootstrapped(e)
	c.Assert(err, gc.ErrorMatches, "environment is already bootstrapped")

	e2 := t.Open(c)
	err = bootstrap.EnsureNotBootstrapped(e2)
	c.Assert(err, gc.ErrorMatches, "environment is already bootstrapped")

	stateServerInstances2, err := e2.StateServerInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(stateServerInstances2, gc.Not(gc.HasLen), 0)
	c.Assert(stateServerInstances2, jc.SameContents, stateServerInstances)

	err = environs.Destroy(e2, t.ConfigStore)
	c.Assert(err, gc.IsNil)

	// Prepare again because Destroy invalidates old environments.
	e3 := t.Prepare(c)

	err = bootstrap.EnsureNotBootstrapped(e3)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(coretesting.Context(c), e3, bootstrap.BootstrapParams{})
	c.Assert(err, gc.IsNil)

	err = bootstrap.EnsureNotBootstrapped(e3)
	c.Assert(err, gc.ErrorMatches, "environment is already bootstrapped")

	err = environs.Destroy(e3, t.ConfigStore)
	c.Assert(err, gc.IsNil)
}
