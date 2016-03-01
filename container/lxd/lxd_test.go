// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	"runtime"
	stdtesting "testing"

	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxd"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/tools/lxdclient"
	jc "github.com/juju/testing/checkers"

	gc "gopkg.in/check.v1"
)

func Test(t *stdtesting.T) {
	if runtime.GOOS == "windows" {
		t.Skip("LXC is not supported on windows")
	}

	/* if there's not a lxd available, don't run the tests */
	/*
		_, err := lxd.ConnectLocal("")
		if err != nil {
			t.Skip("LXD is not avalilable %s", err)
		}
	*/
	gc.TestingT(t)
}

type LxdSuite struct{}

var _ = gc.Suite(&LxdSuite{})

func (t *LxdSuite) makeManager(c *gc.C, name string) container.Manager {
	config := container.ManagerConfig{
		container.ConfigName: name,
	}

	manager, err := lxd.NewContainerManager(config)
	c.Assert(err, jc.ErrorIsNil)

	return manager
}

func (t *LxdSuite) TestNotAllContainersAreDeleted(c *gc.C) {
	c.Skip("Test skipped because it talks directly to LXD agent.")
	lxdClient, err := lxd.ConnectLocal("")
	c.Assert(err, jc.ErrorIsNil)

	/* create a container to make sure isn't deleted */
	instanceSpec := lxdclient.InstanceSpec{
		Name:  "juju-lxd-tests",
		Image: "ubuntu-xenial",
	}

	_, err = lxdClient.AddInstance(instanceSpec)
	c.Assert(err, jc.ErrorIsNil)
	defer lxdClient.RemoveInstances("", "juju-lxd-tests")

	instanceConfig, err := containertesting.MockMachineConfig("1/lxd/0")
	c.Assert(err, jc.ErrorIsNil)
	envConfig, err := config.New(config.NoDefaults, dummy.SampleConfig())
	c.Assert(err, jc.ErrorIsNil)
	instanceConfig.Config = envConfig
	storageConfig := &container.StorageConfig{}
	networkConfig := container.BridgeNetworkConfig("nic42", 4321, nil)

	manager := t.makeManager(c, "manager")
	_, _, err = manager.CreateContainer(
		instanceConfig,
		"xenial",
		networkConfig,
		storageConfig,
	)
	c.Assert(err, jc.ErrorIsNil)

	instances, err := manager.ListContainers()
	c.Assert(err, jc.ErrorIsNil)

	for _, inst := range instances {
		err = manager.DestroyContainer(inst.Id())
		c.Assert(err, jc.ErrorIsNil)
	}
}
