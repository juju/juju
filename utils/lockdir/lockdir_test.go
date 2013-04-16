package lockdir_test

import (
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/utils/lockdir"
)

func Test(t *testing.T) {
	TestingT(t)
}

type lockDirSuite struct{}

var _ = Suite(lockDirSuite{})

func (lockDirSuite) SetUpSuite(c *C) {
	lockdir.SetLockWaitDelay(1 * time.Millisecond)
}

func (lockDirSuite) TearDownSuite(c *C) {
	lockdir.SetLockWaitDelay(1 * time.Second)
}

// This test also happens to test that locks can get created when the lockDir
// doesn't exist.
func (lockDirSuite) TestNamedLockDir(c *C) {
	validLockName := func(name string) {
		dir := c.MkDir()
		_, err := lockdir.NewLock(dir, name)
		c.Assert(err, IsNil)
	}

	validLockName("a")
	validLockName("longer")
	validLockName("longer-with.special-characters")

	invalidLockName := func(name string) {
		dir := c.MkDir()
		_, err := lockdir.NewLock(dir, name)
		c.Assert(err, Equals, lockdir.InvalidLockName)
	}

	invalidLockName("NoCapitals")
	invalidLockName("no+plus")
	invalidLockName("no/slash")
	invalidLockName("no\\backslash")
	invalidLockName("no$dollar")
}

func (lockDirSuite) TestNewLockWithExistingDir(c *C) {
	dir := c.MkDir()
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, IsNil)
	_, err = lockdir.NewLock(dir, "special")
	c.Assert(err, IsNil)
}

func (lockDirSuite) TestNewLockWithExistingFileInPlace(c *C) {
	dir := c.MkDir()
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, IsNil)
	path := path.Join(dir, "locks")
	err = ioutil.WriteFile(path, []byte("foo"), 0644)
	c.Assert(err, IsNil)

	_, err = lockdir.NewLock(path, "special")
	c.Assert(err, ErrorMatches, `lock dir ".*/locks" exists and is a file not a directory`)
}

func (lockDirSuite) TestIsLockHeldBasics(c *C) {
	dir := c.MkDir()
	lock, err := lockdir.NewLock(dir, "testing")
	c.Assert(err, IsNil)
	c.Assert(lock.IsLockHeld(), Equals, false)

	err = lock.Lock()
	c.Assert(err, IsNil)
	c.Assert(lock.IsLockHeld(), Equals, true)

	err = lock.Unlock()
	c.Assert(err, IsNil)
	c.Assert(lock.IsLockHeld(), Equals, false)
}

func (lockDirSuite) TestIsLockHeldTwoLocks(c *C) {
	dir := c.MkDir()
	lock1, err := lockdir.NewLock(dir, "testing")
	c.Assert(err, IsNil)
	lock2, err := lockdir.NewLock(dir, "testing")
	c.Assert(err, IsNil)

	err = lock1.Lock()
	c.Assert(err, IsNil)
	c.Assert(lock2.IsLockHeld(), Equals, false)
}

func (lockDirSuite) TestLockBlocks(c *C) {

	dir := c.MkDir()
	lock1, err := lockdir.NewLock(dir, "testing")
	c.Assert(err, IsNil)
	lock2, err := lockdir.NewLock(dir, "testing")
	c.Assert(err, IsNil)

	acquired := make(chan struct{})
	err = lock1.Lock()
	c.Assert(err, IsNil)

	go func() {
		lock2.Lock()
		acquired <- struct{}{}
		close(acquired)
	}()

	// Waiting for something not to happen is inherintly hard...
	select {
	case <-acquired:
		c.Fatalf("Unexpected lock acquisition")
	case <-time.After(50 * time.Millisecond):
		// all good
	}

	err = lock1.Unlock()
	c.Assert(err, IsNil)

	select {
	case <-acquired:
		// all good
	case <-time.After(50 * time.Millisecond):
		c.Fatalf("Expected lock acquisition")
	}

	c.Assert(lock2.IsLockHeld(), Equals, true)
}
