package fslock_test

import (
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/utils/fslock"
)

func Test(t *testing.T) {
	TestingT(t)
}

type fslockSuite struct{}

var _ = Suite(fslockSuite{})

func (fslockSuite) SetUpSuite(c *C) {
	fslock.SetLockWaitDelay(1 * time.Millisecond)
}

func (fslockSuite) TearDownSuite(c *C) {
	fslock.SetLockWaitDelay(1 * time.Second)
}

// This test also happens to test that locks can get created when the parent
// lock directory doesn't exist.
func (fslockSuite) TestValidNamesLockDir(c *C) {

	for _, name := range []string{
		"a",
		"longer",
		"longer-with.special-characters",
	} {
		dir := c.MkDir()
		_, err := fslock.NewLock(dir, name)
		c.Assert(err, IsNil)
	}
}

func (fslockSuite) TestInvalidNames(c *C) {

	for _, name := range []string{
		"NoCapitals",
		"no+plus",
		"no/slash",
		"no\\backslash",
		"no$dollar",
	} {
		dir := c.MkDir()
		_, err := fslock.NewLock(dir, name)
		c.Assert(err, ErrorMatches, "Invalid lock name .*")
	}
}

func (fslockSuite) TestNewLockWithExistingDir(c *C) {
	dir := c.MkDir()
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, IsNil)
	_, err = fslock.NewLock(dir, "special")
	c.Assert(err, IsNil)
}

func (fslockSuite) TestNewLockWithExistingFileInPlace(c *C) {
	dir := c.MkDir()
	err := os.MkdirAll(dir, 0755)
	c.Assert(err, IsNil)
	path := path.Join(dir, "locks")
	err = ioutil.WriteFile(path, []byte("foo"), 0644)
	c.Assert(err, IsNil)

	_, err = fslock.NewLock(path, "special")
	c.Assert(err, ErrorMatches, `.* not a directory`)
}

func (fslockSuite) TestIsLockHeldBasics(c *C) {
	dir := c.MkDir()
	lock, err := fslock.NewLock(dir, "testing")
	c.Assert(err, IsNil)
	c.Assert(lock.IsLockHeld(), Equals, false)

	err = lock.Lock()
	c.Assert(err, IsNil)
	c.Assert(lock.IsLockHeld(), Equals, true)

	err = lock.Unlock()
	c.Assert(err, IsNil)
	c.Assert(lock.IsLockHeld(), Equals, false)
}

func (fslockSuite) TestIsLockHeldTwoLocks(c *C) {
	dir := c.MkDir()
	lock1, err := fslock.NewLock(dir, "testing")
	c.Assert(err, IsNil)
	lock2, err := fslock.NewLock(dir, "testing")
	c.Assert(err, IsNil)

	err = lock1.Lock()
	c.Assert(err, IsNil)
	c.Assert(lock2.IsLockHeld(), Equals, false)
}

func (fslockSuite) TestLockBlocks(c *C) {

	dir := c.MkDir()
	lock1, err := fslock.NewLock(dir, "testing")
	c.Assert(err, IsNil)
	lock2, err := fslock.NewLock(dir, "testing")
	c.Assert(err, IsNil)

	acquired := make(chan struct{})
	err = lock1.Lock()
	c.Assert(err, IsNil)

	go func() {
		lock2.Lock()
		acquired <- struct{}{}
		close(acquired)
	}()

	// Waiting for something not to happen is inherently hard...
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

func (fslockSuite) TestLockWithTimeoutUnlocked(c *C) {
	dir := c.MkDir()
	lock, err := fslock.NewLock(dir, "testing")
	c.Assert(err, IsNil)

	err = lock.LockWithTimeout(10 * time.Millisecond)
	c.Assert(err, IsNil)
}

func (fslockSuite) TestLockWithTimeoutLocked(c *C) {
	dir := c.MkDir()
	lock1, err := fslock.NewLock(dir, "testing")
	c.Assert(err, IsNil)
	lock2, err := fslock.NewLock(dir, "testing")
	c.Assert(err, IsNil)

	err = lock1.Lock()
	c.Assert(err, IsNil)

	err = lock2.LockWithTimeout(10 * time.Millisecond)
	c.Assert(err, Equals, fslock.ErrTimeout)
}

func (fslockSuite) TestUnlock(c *C) {
	dir := c.MkDir()
	lock, err := fslock.NewLock(dir, "testing")
	c.Assert(err, IsNil)

	err = lock.Unlock()
	c.Assert(err, Equals, fslock.ErrLockNotHeld)
}

func (fslockSuite) TestMessage(c *C) {
	dir := c.MkDir()
	lock, err := fslock.NewLock(dir, "testing")
	c.Assert(err, IsNil)
	c.Assert(lock.GetMessage(), Equals, "")

	err = lock.SetMessage("my message")
	c.Assert(err, Equals, fslock.ErrLockNotHeld)

	err = lock.Lock()
	c.Assert(err, IsNil)

	err = lock.SetMessage("my message")
	c.Assert(err, IsNil)
	c.Assert(lock.GetMessage(), Equals, "my message")

	// Unlocking removes the message.
	err = lock.Unlock()
	c.Assert(err, IsNil)
	c.Assert(lock.GetMessage(), Equals, "")
}

func (fslockSuite) TestMessageAcrossLocks(c *C) {
	dir := c.MkDir()
	lock1, err := fslock.NewLock(dir, "testing")
	c.Assert(err, IsNil)
	lock2, err := fslock.NewLock(dir, "testing")
	c.Assert(err, IsNil)

	err = lock1.Lock()
	c.Assert(err, IsNil)
	err = lock1.SetMessage("very busy")
	c.Assert(err, IsNil)

	c.Assert(lock2.GetMessage(), Equals, "very busy")
}
