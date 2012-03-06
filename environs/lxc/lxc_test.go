package lxc

import (
	. "launchpad.net/gocheck"
	"strings"
	"testing"
)

const (
	DefaultContainer = "lxc_test"
)

func Test(t *testing.T) { TestingT(t) }

type S struct{}

var _ = Suite(&S{})

func (s *S) TestCreate(c *C) {
	var container Container
	container.Name = DefaultContainer

	err := container.Create()
	c.Assert(err, IsNil)

	output := Ls()
	c.Assert(strings.Contains(output, DefaultContainer), Equals, true)

	err = container.Destroy()
	c.Assert(err, IsNil)

	output = Ls()
	c.Assert(strings.Contains(output, DefaultContainer), Equals, false)
}

func (s *S) TestStart(c *C) {
	var container Container
	container.Name = DefaultContainer

	err := container.Create()
	c.Assert(err, IsNil)

	err = container.Start()
	c.Assert(err, IsNil)

	err = container.Stop()
	c.Assert(err, IsNil)

	err = container.Destroy()
	c.Assert(err, IsNil)
}

func (s *S) TestIsRunningWhenContainerIsCreated(c *C) {
	var container Container
	container.Name = DefaultContainer

	err := container.Create()
	c.Assert(err, IsNil)

	c.Assert(container.Running(), Equals, true)

	err = container.Destroy()
	c.Assert(err, IsNil)
}

func (s *S) TestIsNotRunningWhenContainerIsNotCreated(c *C) {
	var container Container
	container.Name = DefaultContainer

	c.Assert(container.Running(), Equals, false)
}

func (s *S) TestRootfs(c *C) {
	var container Container
	container.Name = DefaultContainer
	c.Assert(container.Rootfs(), Equals, "/var/lib/lxc/"+DefaultContainer+"/rootfs/")
}
