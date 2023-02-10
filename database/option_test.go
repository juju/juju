// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/database/app"
	jujutesting "github.com/juju/juju/testing"
)

type optionSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&optionSuite{})

func (s *optionSuite) TestEnsureDataDirSuccess(c *gc.C) {
	subDir := strconv.Itoa(rand.Intn(10))

	cfg := fakeAgentConfig{dataDir: "/tmp/" + subDir}
	f := NewOptionFactory(cfg, stubLogger{})

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
	f := NewOptionFactory(nil, stubLogger{})

	withAddress, err := f.WithAddressOption()
	c.Assert(err, jc.ErrorIsNil)

	dqlite, err := app.New(c.MkDir(), withAddress)
	c.Assert(err, jc.ErrorIsNil)

	_ = dqlite.Close()
}

func (s *optionSuite) TestWithTLSOptionSuccess(c *gc.C) {
	cfg := fakeAgentConfig{}
	f := NewOptionFactory(cfg, stubLogger{})

	withTLS, err := f.WithTLSOption()
	c.Assert(err, jc.ErrorIsNil)

	dqlite, err := app.New(c.MkDir(), withTLS)
	c.Assert(err, jc.ErrorIsNil)

	_ = dqlite.Close()
}

func (s *optionSuite) TestWithClusterOptionSuccess(c *gc.C) {
	// Hack to get a bind address to add to config.
	h := NewOptionFactory(fakeAgentConfig{}, stubLogger{})
	err := h.ensureBindAddress()
	c.Assert(err, jc.ErrorIsNil)

	cfg := fakeAgentConfig{
		apiAddrs: []string{
			"10.0.0.5:17070",
			h.bindAddress,     // Filtered out as not being us.
			"127.0.0.1:17070", // Filtered out as a non-local-cloud address.
		},
	}

	f := NewOptionFactory(cfg, stubLogger{})

	withCluster, err := f.WithClusterOption()
	c.Assert(err, jc.ErrorIsNil)

	dqlite, err := app.New(c.MkDir(), withCluster)
	c.Assert(err, jc.ErrorIsNil)

	_ = dqlite.Close()
}

func (s *optionSuite) TestWithClusterNotHASuccess(c *gc.C) {
	// Hack to get a bind address to add to config.
	h := NewOptionFactory(fakeAgentConfig{}, stubLogger{})
	err := h.ensureBindAddress()
	c.Assert(err, jc.ErrorIsNil)

	cfg := fakeAgentConfig{apiAddrs: []string{h.bindAddress}}

	f := NewOptionFactory(cfg, stubLogger{})

	withCluster, err := f.WithClusterOption()
	c.Assert(err, jc.ErrorIsNil)

	dqlite, err := app.New(c.MkDir(), withCluster)
	c.Assert(err, jc.ErrorIsNil)

	_ = dqlite.Close()
}

func (s *optionSuite) TestIgnoreInterface(c *gc.C) {
	shouldIgnore := []string{
		"lxdbr0",
		"virbr0",
		"docker0",
	}
	for _, devName := range shouldIgnore {
		c.Check(ignoreInterface(net.Interface{Name: devName}), jc.IsTrue)
	}

	c.Check(ignoreInterface(net.Interface{Flags: net.FlagLoopback}), jc.IsTrue)
	c.Check(ignoreInterface(net.Interface{Name: "enp5s0"}), jc.IsFalse)
}

type fakeAgentConfig struct {
	agent.Config

	dataDir  string
	apiAddrs []string
}

// DataDir implements agent.Config.
func (cfg fakeAgentConfig) DataDir() string {
	return cfg.dataDir
}

// CACert implements agent.Config.
func (cfg fakeAgentConfig) CACert() string {
	return jujutesting.CACert
}

// StateServingInfo implements agent.AgentConfig.
func (cfg fakeAgentConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	return controller.StateServingInfo{
		CAPrivateKey: jujutesting.CAKey,
		Cert:         jujutesting.ServerCert,
		PrivateKey:   jujutesting.ServerKey,
	}, true
}

// APIAddresses implements agent.Config.
func (cfg fakeAgentConfig) APIAddresses() ([]string, error) {
	return cfg.apiAddrs, nil
}
