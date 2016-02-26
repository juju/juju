// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujutest

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

// Tests is a gocheck suite containing tests verifying juju functionality
// against the environment with the given configuration. The
// tests are not designed to be run against a live server - the Environ
// is opened once for each test, and some potentially expensive operations
// may be executed.
type Tests struct {
	TestConfig    coretesting.Attrs
	Credential    cloud.Credential
	CloudEndpoint string
	CloudRegion   string
	envtesting.ToolsFixture
	sstesting.TestDataSuite
	// ConfigStore holds the configuration storage
	// used when preparing the environment.
	// This is initialized by SetUpTest.
	ConfigStore configstore.Storage

	// ControllerStore holds the controller related informtion
	// such as controllers, accounts, etc., used when preparing
	// the environment. This is initialized by SetUpSuite.
	ControllerStore jujuclient.ClientStore
}

// Open opens an instance of the testing environment.
func (t *Tests) Open(c *gc.C) environs.Environ {
	modelName := t.TestConfig["name"].(string)
	info, err := t.ConfigStore.ReadInfo(configstore.EnvironInfoName(modelName, modelName))
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := config.New(config.NoDefaults, info.BootstrapConfig())
	c.Assert(err, jc.ErrorIsNil)
	e, err := environs.New(cfg)
	c.Assert(err, gc.IsNil, gc.Commentf("opening environ %#v", cfg.AllAttrs()))
	c.Assert(e, gc.NotNil)
	return e
}

// Prepare prepares an instance of the testing environment.
func (t *Tests) Prepare(c *gc.C) environs.Environ {
	cfg, err := config.New(config.NoDefaults, t.TestConfig)
	c.Assert(err, jc.ErrorIsNil)
	credential := t.Credential
	if credential.AuthType() == "" {
		credential = cloud.NewEmptyCredential()
	}
	args := environs.PrepareForBootstrapParams{
		Config:        cfg,
		Credentials:   credential,
		CloudEndpoint: t.CloudEndpoint,
		CloudRegion:   t.CloudRegion,
	}
	e, err := environs.Prepare(envtesting.BootstrapContext(c), t.ConfigStore, t.ControllerStore, args.Config.Name(), args)
	c.Assert(err, gc.IsNil, gc.Commentf("preparing environ %#v", t.TestConfig))
	c.Assert(e, gc.NotNil)
	return e
}

func (t *Tests) SetUpTest(c *gc.C) {
	storageDir := c.MkDir()
	t.DefaultBaseURL = "file://" + storageDir + "/tools"
	t.ToolsFixture.SetUpTest(c)
	t.UploadFakeToolsToDirectory(c, storageDir, "released", "released")
	t.ConfigStore = configstore.NewMem()
	t.ControllerStore = jujuclienttesting.NewMemStore()
}

func (t *Tests) TearDownTest(c *gc.C) {
	t.ToolsFixture.TearDownTest(c)
}

func (t *Tests) TestStartStop(c *gc.C) {
	e := t.Prepare(c)
	cfg, err := e.Config().Apply(map[string]interface{}{
		"agent-version": version.Current.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = e.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)

	insts, err := e.Instances(nil)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 2)
	c.Assert(insts[0].Id(), gc.Equals, id0)
	c.Assert(insts[1].Id(), gc.Equals, id1)

	// order of results is not specified
	insts, err = e.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 2)
	c.Assert(insts[0].Id(), gc.Not(gc.Equals), insts[1].Id())

	err = e.StopInstances(inst0.Id())
	c.Assert(err, jc.ErrorIsNil)

	insts, err = e.Instances([]instance.Id{id0, id1})
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(insts[0], gc.IsNil)
	c.Assert(insts[1].Id(), gc.Equals, id1)

	insts, err = e.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts[0].Id(), gc.Equals, id1)
}

func (t *Tests) TestBootstrap(c *gc.C) {
	e := t.Prepare(c)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), e, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	controllerInstances, err := e.ControllerInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerInstances, gc.Not(gc.HasLen), 0)

	e2 := t.Open(c)
	controllerInstances2, err := e2.ControllerInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(controllerInstances2, gc.Not(gc.HasLen), 0)
	c.Assert(controllerInstances2, jc.SameContents, controllerInstances)

	err = environs.Destroy(e2.Config().Name(), e2, t.ConfigStore, t.ControllerStore)
	c.Assert(err, jc.ErrorIsNil)

	// Prepare again because Destroy invalidates old environments.
	e3 := t.Prepare(c)

	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), e3, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = environs.Destroy(e3.Config().Name(), e3, t.ConfigStore, t.ControllerStore)
	c.Assert(err, jc.ErrorIsNil)
}
