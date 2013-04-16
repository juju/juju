package lockdir_test

import (
	"io/ioutil"
	"os"
	"path"
	"testing"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/utils/lockdir"
)

func Test(t *testing.T) {
	TestingT(t)
}

type lockDirSuite struct{}

var _ = Suite(lockDirSuite{})

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
