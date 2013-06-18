// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/instance"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	. "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/version"
)

type lxcBrokerSuite struct {
	testing.LoggingSuite
	containerDir       string
	removedDir         string
	lxcDir             string
	oldContainerDir    string
	oldRemovedDir      string
	oldLxcContainerDir string
}

var _ = Suite(&lxcBrokerSuite{})

func (s *lxcBrokerSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *lxcBrokerSuite) TearDownSuite(c *C) {
	s.LoggingSuite.TearDownSuite(c)
}

func (s *lxcBrokerSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.containerDir = c.MkDir()
	s.oldContainerDir = lxc.SetContainerDir(s.containerDir)
	s.removedDir = c.MkDir()
	s.oldRemovedDir = lxc.SetRemovedContainerDir(s.removedDir)
	s.lxcDir = c.MkDir()
	s.oldLxcContainerDir = lxc.SetLxcContainerDir(s.lxcDir)
}

func (s *lxcBrokerSuite) TearDownTest(c *C) {
	lxc.SetContainerDir(s.oldContainerDir)
	lxc.SetLxcContainerDir(s.oldLxcContainerDir)
	lxc.SetRemovedContainerDir(s.oldRemovedDir)
	s.LoggingSuite.TearDownTest(c)
}

func (*lxcBrokerSuite) TestInstanceInterface(c *C) {

}
