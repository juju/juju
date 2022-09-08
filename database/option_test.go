// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"net"

	"github.com/juju/juju/agent"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type optionSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&optionSuite{})

func (s *optionSuite) TestWithAddressOption(c *gc.C) {
	cfg := dummyAgentConfig{dataDir: "/tmp/dqlite"}

	f := NewOptionFactory(cfg, dqlitePort, func() ([]net.Addr, error) {
		return []net.Addr{
			&net.IPAddr{IP: net.ParseIP("10.0.0.5")},
			&net.IPAddr{IP: net.ParseIP("127.0.0.1")},
		}, nil
	})

	// We can not actually test the realisation of this option,
	// as the options type from the go-dqlite is not exported.
	// We are also unable to test by creating a new dqlite app,
	// because it fails to bind to the contrived address.
	// The best we can do is check that we selected an address
	// based on the absence of an error.
	_, err := f.WithAddressOption()
	c.Assert(err, jc.ErrorIsNil)
}

type dummyAgentConfig struct {
	agent.Config
	dataDir string
}

// DataDir implements agent.AgentConfig.
func (cfg dummyAgentConfig) DataDir() string {
	return cfg.dataDir
}
