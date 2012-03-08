// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.
package state

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/gozk/zookeeper"
	"os"
	"testing"
)

// ZkAddr is the address for the connection to the server.
var ZkAddr string

// ZkSetUpEnvironment initializes the ZooKeeper test environment.
func ZkSetUpEnvironment(t *testing.T) (*zookeeper.Server, string) {
	dir, err := ioutil.TempDir("", "statetest")
	if err != nil {
		t.Fatalf("cannot create temporary directory: %v", err)
	}
	testRoot := dir + "/zookeeper"
	testPort := 21812
	srv, err := zookeeper.CreateServer(testPort, testRoot, "")
	if err != nil {
		t.Fatalf("cannot create ZooKeeper server: %v", err)
	}
	err = srv.Start()
	if err != nil {
		t.Fatalf("cannot start ZooKeeper server: %v", err)
	}
	ZkAddr = fmt.Sprint("localhost:", testPort)
	return srv, dir
}

// ZkTearDownEnvironment destroys the ZooKeeper test environment.
func ZkTearDownEnvironment(t *testing.T, srv *zookeeper.Server, dir string) {
	srv.Destroy()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal("cannot remove temporary directory: %v", err)
	}
}

// ZkConn returns the ZooKeeper connection used by a state.
// It is defined in export_test.go so that tests can have access
// to this connection.
func ZkConn(st *State) *zookeeper.Conn {
	return st.zk
}

// DummyEntity is a helper to test the agent interface and the
// helper functions to realize it. Those functions are only
// visible inside the state package.
type DummyEntity struct {
	st *State
}

func NewDummyEntity(st *State) *DummyEntity {
	return &DummyEntity{st}
}

func (d *DummyEntity) Key() string {
	return "key-0000000001"
}

func (d *DummyEntity) zkAgentPath() string {
	return fmt.Sprintf("/dummy/%s/agent", d.Key())
}

func (d *DummyEntity) HasAgent() (bool, error) {
	return hasAgent(d.st, d.zkAgentPath())
}

func (d *DummyEntity) WatchAgent() *AgentWatcher {
	return watchAgent(d.st, d.zkAgentPath())
}

func (d *DummyEntity) ConnectAgent() error {
	return connectAgent(d.st, d.zkAgentPath())
}

var _ = Agent(&DummyEntity{})
