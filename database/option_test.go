// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"

	"github.com/canonical/go-dqlite/app"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	jujutesting "github.com/juju/juju/testing"
)

type optionSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&optionSuite{})

func (s *optionSuite) TestEnsureDataDir(c *gc.C) {
	subDir := strconv.Itoa(rand.Intn(10))

	cfg := fakeAgentConfig{dataDir: "/tmp/" + subDir}
	f := NewOptionFactory(cfg)

	expected := fmt.Sprintf("/tmp/%s/%s", subDir, dqliteDataDir)
	s.AddCleanup(func(*gc.C) { _ = os.RemoveAll(cfg.DataDir()) })

	// Call twice to check both the creation and extant scenarios.
	dir, err := f.EnsureDataDir()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(dir, gc.Equals, expected)

	_, err = os.Stat(expected)
	c.Assert(err, jc.ErrorIsNil)

	dir, err = f.EnsureDataDir()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(dir, gc.Equals, expected)

	_, err = os.Stat(expected)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *optionSuite) TestWithAddressOptionSuccess(c *gc.C) {
	f := NewOptionFactory(nil)

	f.interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{
			&net.IPAddr{IP: net.ParseIP("10.0.0.5")},
			&net.IPAddr{IP: net.ParseIP("127.0.0.1")},
		}, nil
	}

	// We can not actually test the realisation of this option,
	// as the options type from the go-dqlite is not exported.
	// We are also unable to test by creating a new dqlite app,
	// because it fails to bind to the contrived address.
	// The best we can do is check that we selected an address
	// based on the absence of an error.
	_, err := f.WithAddressOption()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *optionSuite) TestWithAddressOptionMultipleAddressError(c *gc.C) {
	f := NewOptionFactory(nil)

	f.interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{
			&net.IPAddr{IP: net.ParseIP("10.0.0.5")},
			&net.IPAddr{IP: net.ParseIP("10.0.0.6")},
		}, nil
	}

	_, err := f.WithAddressOption()
	c.Assert(err, gc.ErrorMatches,
		`.* found \[local-cloud:10.0.0.5 local-cloud:10.0.0.6\]`)
}

func (s *optionSuite) TestWithTLSOption(c *gc.C) {
	cfg := fakeAgentConfig{}
	f := NewOptionFactory(cfg)

	withTLS, err := f.WithTLSOption()
	c.Assert(err, jc.ErrorIsNil)

	dqlite, err := app.New(c.MkDir(), withTLS)
	c.Assert(err, jc.ErrorIsNil)

	_ = dqlite.Close()
}

type fakeAgentConfig struct {
	agent.Config

	dataDir string
}

// DataDir implements agent.AgentConfig.
func (cfg fakeAgentConfig) DataDir() string {
	return cfg.dataDir
}

func (cfg fakeAgentConfig) CACert() string {
	return jujutesting.CACert
}

func (cfg fakeAgentConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	return controller.StateServingInfo{
		CAPrivateKey: jujutesting.CAKey,
		Cert:         jujutesting.ServerCert,
		PrivateKey:   jujutesting.ServerKey,
	}, true
}
