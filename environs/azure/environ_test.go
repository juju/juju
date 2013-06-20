// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
	"sync"
)

type EnvironSuite struct {
	ProviderSuite
}

var _ = Suite(new(EnvironSuite))

func makeEnviron(c *C) *azureEnviron {
	attrs := makeAzureConfigMap(c)
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)
	ecfg, err := azureEnvironProvider{}.newConfig(cfg)
	c.Assert(err, IsNil)
	return &azureEnviron{
		name: "env",
		ecfg: ecfg,
	}
}

func (EnvironSuite) TestGetSnapshot(c *C) {
	original := azureEnviron{name: "this-env", ecfg: new(azureEnvironConfig)}
	snapshot := original.getSnapshot()

	// The snapshot is identical to the original.
	c.Check(*snapshot, DeepEquals, original)

	// However, they are distinct objects.
	c.Check(snapshot, Not(Equals), &original)

	// It's a shallow copy; they still share pointers.
	c.Check(snapshot.ecfg, Equals, original.ecfg)

	// Neither object is locked at the end of the copy.
	c.Check(original.Mutex, Equals, sync.Mutex{})
	c.Check(snapshot.Mutex, Equals, sync.Mutex{})
}

func (EnvironSuite) TestGetSnapshotLocksEnviron(c *C) {
	original := azureEnviron{}
	testing.TestLockingFunction(&original.Mutex, func() { original.getSnapshot() })
}

func (EnvironSuite) TestName(c *C) {
	env := azureEnviron{name: "foo"}
	c.Check(env.Name(), Equals, env.name)
}

func (EnvironSuite) TestConfigReturnsConfig(c *C) {
	cfg := new(config.Config)
	ecfg := azureEnvironConfig{Config: cfg}
	env := azureEnviron{ecfg: &ecfg}
	c.Check(env.Config(), Equals, cfg)
}

func (EnvironSuite) TestConfigLocksEnviron(c *C) {
	env := azureEnviron{name: "env", ecfg: new(azureEnvironConfig)}
	testing.TestLockingFunction(&env.Mutex, func() { env.Config() })
}

// TODO: Temporarily deactivating this code.  Passing certificate in-memory
// may require gwacl change.
/*
func (EnvironSuite) TestGetManagementAPI(c *C) {
	env := makeEnviron(c)
	management, err := env.getManagementAPI()
	c.Assert(err, IsNil)
	c.Check(management, NotNil)
}
*/

func (EnvironSuite) TestGetStorageContext(c *C) {
	env := makeEnviron(c)
	storage, err := env.getStorageContext()
	c.Assert(err, IsNil)
	c.Assert(storage, NotNil)
	c.Check(storage.Account, Equals, env.ecfg.StorageAccountName())
	c.Check(storage.Key, Equals, env.ecfg.StorageAccountKey())
}
