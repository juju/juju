// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"path"
	"strconv"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/database/app"
	"github.com/juju/juju/database/dqlite"
	jujutesting "github.com/juju/juju/testing"
)

type nodeManagerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&nodeManagerSuite{})

func (s *nodeManagerSuite) TestEnsureDataDirSuccess(c *gc.C) {
	subDir := strconv.Itoa(rand.Intn(10))

	cfg := fakeAgentConfig{dataDir: "/tmp/" + subDir}
	m := NewNodeManager(cfg, stubLogger{})

	expected := fmt.Sprintf("/tmp/%s/%s", subDir, dqliteDataDir)
	s.AddCleanup(func(*gc.C) { _ = os.RemoveAll(cfg.DataDir()) })

	// Call twice to check both the creation and extant scenarios.
	dir, err := m.EnsureDataDir()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(dir, gc.Equals, expected)

	_, err = os.Stat(expected)
	c.Assert(err, jc.ErrorIsNil)

	dir, err = m.EnsureDataDir()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(dir, gc.Equals, expected)

	_, err = os.Stat(expected)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *nodeManagerSuite) TestIsExistingNode(c *gc.C) {
	subDir := strconv.Itoa(rand.Intn(10))

	cfg := fakeAgentConfig{dataDir: "/tmp/" + subDir}
	s.AddCleanup(func(*gc.C) { _ = os.RemoveAll(cfg.DataDir()) })

	m := NewNodeManager(cfg, stubLogger{})

	// Empty directory indicates we've never started.
	extant, err := m.IsExistingNode()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(extant, jc.IsFalse)

	// Non-empty indicates we've come up before.
	dataDir, err := m.EnsureDataDir()
	c.Assert(err, jc.ErrorIsNil)

	someFile := path.Join(dataDir, "a-file.txt")
	err = os.WriteFile(someFile, nil, 06000)
	c.Assert(err, jc.ErrorIsNil)

	extant, err = m.IsExistingNode()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(extant, jc.IsTrue)
}

func (s *nodeManagerSuite) TestIsBootstrappedNode(c *gc.C) {
	subDir := strconv.Itoa(rand.Intn(10))

	cfg := fakeAgentConfig{dataDir: "/tmp/" + subDir}
	s.AddCleanup(func(*gc.C) { _ = os.RemoveAll(cfg.DataDir()) })

	m := NewNodeManager(cfg, stubLogger{})
	ctx := context.TODO()

	// Empty directory indicates we are not the bootstrapped node.
	asBootstrapped, err := m.IsBootstrappedNode(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(asBootstrapped, jc.IsFalse)

	dataDir, err := m.EnsureDataDir()
	c.Assert(err, jc.ErrorIsNil)

	clusterFile := path.Join(dataDir, dqliteClusterFileName)

	// Multiple nodes indicates the cluster has mutated since bootstrap.
	data := `
- Address: 10.246.27.114:17666
  ID: 3297041220608546238
  Role: 0
- Address: 10.246.27.115:17666
  ID: 123456789
  Role: 0
`[1:]

	err = os.WriteFile(clusterFile, []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)

	asBootstrapped, err = m.IsBootstrappedNode(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(asBootstrapped, jc.IsFalse)

	// Non-loopback address indicates node was reconfigured since bootstrap.
	data = `
- Address: 10.246.27.114:17666
  ID: 3297041220608546238
  Role: 0
`[1:]

	err = os.WriteFile(clusterFile, []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)

	asBootstrapped, err = m.IsBootstrappedNode(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(asBootstrapped, jc.IsFalse)

	// Loopback IP address indicates the node is as we bootstrapped it.
	data = `
- Address: 127.0.0.1:17666
  ID: 3297041220608546238
  Role: 0
`[1:]

	err = os.WriteFile(clusterFile, []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)

	asBootstrapped, err = m.IsBootstrappedNode(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(asBootstrapped, jc.IsTrue)
}

func (s *nodeManagerSuite) TestSetClusterServersSuccess(c *gc.C) {
	subDir := strconv.Itoa(rand.Intn(10))

	cfg := fakeAgentConfig{dataDir: "/tmp/" + subDir}
	s.AddCleanup(func(*gc.C) { _ = os.RemoveAll(cfg.DataDir()) })

	m := NewNodeManager(cfg, stubLogger{})
	ctx := context.TODO()

	dataDir, err := m.EnsureDataDir()
	c.Assert(err, jc.ErrorIsNil)

	clusterFile := path.Join(dataDir, dqliteClusterFileName)

	// Write a cluster.yaml file into the Dqlite data directory.
	data := []byte(`
- Address: 127.0.0.1:17666
  ID: 3297041220608546238
  Role: 0
`[1:])

	err = os.WriteFile(clusterFile, data, 0600)
	c.Assert(err, jc.ErrorIsNil)

	servers := []dqlite.NodeInfo{
		{
			ID:      3297041220608546238,
			Address: "10.6.6.6:17666",
			Role:    0,
		},
	}

	err = m.SetClusterServers(ctx, servers)
	c.Assert(err, jc.ErrorIsNil)

	data, err = os.ReadFile(clusterFile)
	c.Assert(err, jc.ErrorIsNil)

	// cluster.yaml should reflect the new server list.
	c.Check(string(data), gc.Equals, `
- Address: 10.6.6.6:17666
  ID: 3297041220608546238
  Role: 0
`[1:])
}

func (s *nodeManagerSuite) TestSetNodeInfoSuccess(c *gc.C) {
	subDir := strconv.Itoa(rand.Intn(10))

	cfg := fakeAgentConfig{dataDir: "/tmp/" + subDir}
	s.AddCleanup(func(*gc.C) { _ = os.RemoveAll(cfg.DataDir()) })

	m := NewNodeManager(cfg, stubLogger{})
	dataDir, err := m.EnsureDataDir()
	c.Assert(err, jc.ErrorIsNil)

	infoFile := path.Join(dataDir, "info.yaml")

	// Write a cluster.yaml file into the Dqlite data directory.
	data := []byte(`
Address: 127.0.0.1:17666
ID: 3297041220608546238
Role: 0
`[1:])

	err = os.WriteFile(infoFile, data, 0600)
	c.Assert(err, jc.ErrorIsNil)

	server := dqlite.NodeInfo{
		ID:      3297041220608546238,
		Address: "10.6.6.6:17666",
		Role:    0,
	}

	err = m.SetNodeInfo(server)
	c.Assert(err, jc.ErrorIsNil)

	data, err = os.ReadFile(infoFile)
	c.Assert(err, jc.ErrorIsNil)

	// info.yaml should reflect the new node info.
	c.Check(string(data), gc.Equals, `
Address: 10.6.6.6:17666
ID: 3297041220608546238
Role: 0
`[1:])
}

func (s *nodeManagerSuite) TestWithAddressOptionSuccess(c *gc.C) {
	m := NewNodeManager(nil, stubLogger{})

	withAddress, err := m.WithAddressOption()
	c.Assert(err, jc.ErrorIsNil)

	dqlite, err := app.New(c.MkDir(), withAddress)
	c.Assert(err, jc.ErrorIsNil)

	_ = dqlite.Close()
}

func (s *nodeManagerSuite) TestWithTLSOptionSuccess(c *gc.C) {
	cfg := fakeAgentConfig{}
	m := NewNodeManager(cfg, stubLogger{})

	withTLS, err := m.WithTLSOption()
	c.Assert(err, jc.ErrorIsNil)

	dqlite, err := app.New(c.MkDir(), withTLS)
	c.Assert(err, jc.ErrorIsNil)

	_ = dqlite.Close()
}

func (s *nodeManagerSuite) TestWithClusterOptionSuccess(c *gc.C) {
	// Hack to get a bind address to add to config.
	h := NewNodeManager(fakeAgentConfig{}, stubLogger{})
	err := h.ensureBindAddress()
	c.Assert(err, jc.ErrorIsNil)

	cfg := fakeAgentConfig{
		apiAddrs: []string{
			"10.0.0.5:17070",
			h.bindAddress,     // Filtered out as not being us.
			"127.0.0.1:17070", // Filtered out as a non-local-cloud address.
		},
	}

	m := NewNodeManager(cfg, stubLogger{})

	withCluster, err := m.WithClusterOption()
	c.Assert(err, jc.ErrorIsNil)

	dqlite, err := app.New(c.MkDir(), withCluster)
	c.Assert(err, jc.ErrorIsNil)

	_ = dqlite.Close()
}

func (s *nodeManagerSuite) TestWithClusterNotHASuccess(c *gc.C) {
	// Hack to get a bind address to add to config.
	h := NewNodeManager(fakeAgentConfig{}, stubLogger{})
	err := h.ensureBindAddress()
	c.Assert(err, jc.ErrorIsNil)

	cfg := fakeAgentConfig{apiAddrs: []string{h.bindAddress}}

	m := NewNodeManager(cfg, stubLogger{})

	withCluster, err := m.WithClusterOption()
	c.Assert(err, jc.ErrorIsNil)

	dqlite, err := app.New(c.MkDir(), withCluster)
	c.Assert(err, jc.ErrorIsNil)

	_ = dqlite.Close()
}

func (s *nodeManagerSuite) TestIgnoreInterface(c *gc.C) {
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
