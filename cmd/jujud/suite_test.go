package main

import (
	. "launchpad.net/gocheck"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	stdtesting "testing"
)

func TestPackage(t *stdtesting.T) {
	testing.ZkTestPackage(t)
}

type zkSuite struct {
	testing.ZkConnSuite
	zkInfo *state.Info
}

func (s *zkSuite) SetUpSuite(c *C) {
	s.ZkConnSuite.SetUpSuite(c)
	s.zkInfo = &state.Info{Addrs: []string{testing.ZkAddr}}
}
