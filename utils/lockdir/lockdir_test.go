package lockdir_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/utils/lockdir"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type lockDirSuite struct{}

var _ = Suite(lockDirSuite{})

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
