// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"errors"
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

// testLockingFunction verifies that a function obeys a given lock.
//
// Use this as a building block in your own tests for proper locking.
// Parameters are a gocheck object to run assertions on; the lock that you
// expect your function to block on; and the function that you want to test
// for proper locking on that lock.
func testLockingFunction(lock *sync.Mutex, function func()) {
	// We record two events that must happen in the right order.
	// Buffer the channel so that we don't get hung up during attempts
	// to push the events in.
	events := make(chan string, 2)
	// Synchronization channel, to make sure that the function starts
	// trying to run at the point where we're going to make it block.
	proceed := make(chan bool)

	goroutine := func() {
		proceed <- true
		function()
		events <- "complete function"
	}

	lock.Lock()
	go goroutine()
	// Make the goroutine start here.  It should block in "function()."
	<-proceed

	// TODO: In Go 1.1, call runtime.GoSched a few times to give a
	// misbehaved "function" plenty of rope to hang itself.

	events <- "release lock"
	lock.Unlock()

	// Now that we've released the lock, the function is unblocked.  Read
	// the 2 events.  (This will wait until the function has completed.)
	firstEvent := <-events
	secondEvent := <-events
	if firstEvent != "release lock" || secondEvent != "complete function" {
		panic(errors.New("function did not obey lock"))
	}

	// Also, the function must have released the lock.
	blankLock := sync.Mutex{}
	if *lock != blankLock {
		panic(errors.New("function did not release lock"))
	}
}

func (EnvironSuite) TestTestLockingFunctionPassesCorrectLock(c *C) {
	lock := sync.Mutex{}
	function := func() {
		lock.Lock()
		lock.Unlock()
	}
	// testLockingFunction succeeds.
	testLockingFunction(&lock, function)
}

func (EnvironSuite) TestTestLockingFunctionDetectsDisobeyedLock(c *C) {
	lock := sync.Mutex{}
	function := func() {}
	c.Check(
		func() { testLockingFunction(&lock, function) },
		Panics,
		errors.New("function did not obey lock"))
}

func (EnvironSuite) TestTestLockingFunctionDetectsFailureToReleaseLock(c *C) {
	lock := sync.Mutex{}
	defer lock.Unlock()
	function := func() {
		lock.Lock()
	}
	c.Check(
		func() { testLockingFunction(&lock, function) },
		Panics,
		errors.New("function did not release lock"))
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
	testLockingFunction(&original.Mutex, func() { original.getSnapshot() })
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
	testLockingFunction(&env.Mutex, func() { env.Config() })
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
