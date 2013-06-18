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

// A note on locking tests.  Proper locking is hard to test for.  Tests here
// use a fixed pattern to verify that a function obeys a particular lock:
//
// 1. Create a channel for the function's result.
// 2. Grab the lock.
// 3. Launch goroutine 1: invoke the function and report result to the channel.
// 4. Launch goroutine 2: modify the object and then release the lock.
// 5. Retrieve result from the channel.
// 6. Test that the result reflects goroutine 2's modification.
// 7. Test that the lock was released in the end.
//
// If the function obeys the lock, it can't complete until goroutine 2 has
// completed.  If it doesn't, it can.  The pattern aims for this scenario:
//
//  The mainline code blocks on the channel.
//  Goroutine 1 starts.  It invokes the function you want to test.
//  The function tries to grab the lock, and blocks.
//  Goroutine 2 starts.  It releases the lock and exits.
//  The function in goroutine 1 is now unblocked.
//
// It would be simpler to have just one goroutine (and skip the channel), and
// release the lock inline.  But then the ordering depends on a more
// fundamental choice within the language implementation: it may choose to
// start running a goroutine immediately at the "go" statement, or it may
// continue executing the inline code and postpone execution of the goroutine
// until the inline code blocks.
//
// The pattern is still not a full guarantee that the lock is obeyed.  The
// language implementation might choose to run the goroutines in LIFO order,
// and then the locking would not be exercised.  The lock would simply be
// available by the time goroutine 1 ran, and the test would never fail unless
// the function you're testing neglected to release the lock.  But as long as
// there is a reasonable chance of the first goroutine starting before the
// second, there is a chance of exposing a function that disobeys the lock.

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
	// This tests follows the locking-test pattern.  See comment above.
	// If you want to change how this works, you probably want to update
	// any other tests with the same pattern as well.
	original := azureEnviron{name: "old-name"}
	// 1. Result comes out of this channel.
	snaps := make(chan *azureEnviron)
	// 2. Stop a well-behaved getSnapshot from running (for now).
	original.Lock()
	// 3. Goroutine 1: ask for a snapshot.  The point of the test is that
	// this blocks until we release our lock.
	go func() {
		snaps <- original.getSnapshot()
	}()
	// 4. Goroutine 2: release the lock.  The getSnapshot call can't
	// complete until we've done this.
	go func() {
		original.name = "new-name"
		original.Unlock()
	}()
	// 5. Let the goroutines do their work.
	snapshot := <-snaps
	// 6. Test: the snapshot was made only after the lock was released.
	c.Check(snapshot.name, Equals, "new-name")
	// 7. Test: getSnapshot released the lock.
	c.Check(original.Mutex, Equals, sync.Mutex{})
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
	// This tests follows the locking-test pattern.  See comment above.
	// If you want to change how this works, you probably want to update
	// any other tests with the same pattern as well.
	env := azureEnviron{name: "env", ecfg: new(azureEnvironConfig)}
	newConfig := new(config.Config)
	// 1. Create results channel.
	configs := make(chan *config.Config)
	// 2. Stop a well-behaved Config() from running, for now.
	env.Lock()
	// 3. Goroutine 1: call Config().  We want to test that this locks.
	go func() {
		configs <- env.Config()
	}()
	// 4. Goroutine 2: change the Environ object, and release the lock.
	go func() {
		env.ecfg = &azureEnvironConfig{Config: newConfig}
		env.Unlock()
	}()
	// 5. Let the goroutines do their work.
	config := <-configs
	// 6. Test that goroutine 2 completed before Config did.
	c.Check(config, Equals, newConfig)
	// 7. Test: Config() released the lock.
	c.Check(env.Mutex, Equals, sync.Mutex{})
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
