// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
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

// exerciseLockingFunction verifies that a function obeys a given lock.
//
// Use this as a building block in your own tests for proper locking.
// Parameters are the lock that you expect your function to block on; the
// function that you want to test for proper locking on that lock; and a
// marker callback whose effect you can observe.
//
// There needs to be a testable interaction between your "function" and your
// "marker," such that you can see from the result of "function" which of the
// two executed first.  So the "marker" must make a change that your "function"
// can observe, and the "function" must return a result that you can look at
// later to see whether it ran before or after the "marker."  You will get that
// result back as the return value.
//
// If the function that you're actually testing does not return anything that
// you can check for the marker effect, then pass a wrapper function that first
// invokes the function whose locking you want to test, and then records a
// result that shows whether the marker has run.
//
// After calling this function, you should test that:
// 1. The lock has been released.
// 2. Your "marker" function was completed before "function" executed.
//
// The "marker" will be executed while holding "lock."
//
// The return value of "function" must be suitable for transmission through a
// channel.
func exerciseLockingFunction(lock sync.Locker, function func() interface{}, marker func()) interface{} {
	// Here's the scenario this test aims for:
	// 1. We grab the lock.
	// 2. Goroutine invokes "function."
	// 3. Since the lock is not available, goroutine should block there.
	// 4. Execute "marker," while "function" is still blocked.
	// 5. Release the lock.
	// 6. Now execution of "function" can complete.
	//
	// If "function" blocks on "lock," then you should see the effect of
	// "marker" already present in the result.  If it doesn't, then
	// "function" will execute straight through.  In single-thread
	// execution you'll see a result that predates execution of the marker,
	// or in multi-thread execution the result will be indeterminate.
	result := make(chan interface{})
	proceed := make(chan bool)

	lock.Lock()
	go func() {
		proceed <- true
		result <- function()
	}()

	// Wait for the goroutine to start.  It should get stuck on "function."
	<-proceed

	// TODO: In Go 1.1, call runtime.GoSched a few times to give a
	// misbehaved "function" plenty of rope to hang itself.

	// Run the marker, while still holding the lock.  If "function" is
	// well-behaved, it's still waiting for us to do this.
	marker()
	// And now, finally, allow "function" to complete.
	lock.Unlock()

	return <-result
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
	original := azureEnviron{name: "old-name"}
	snapshot := exerciseLockingFunction(
		&original,
		func() interface{} { return original.getSnapshot() },
		func() { original.name = "new-name" })

	c.Check(original.Mutex, Equals, sync.Mutex{})
	c.Check(snapshot.(*azureEnviron).name, Equals, "new-name")
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
	newConfig := new(config.Config)
	config := exerciseLockingFunction(
		&env,
		func() interface{} { return env.Config() },
		func() { env.ecfg = &azureEnvironConfig{Config: newConfig} })
	c.Check(env.Mutex, Equals, sync.Mutex{})
	c.Check(config, Equals, newConfig)
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
