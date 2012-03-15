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

// AgentMixinEntity is a helper representing any state entity which
// implements the agent processable interface. It uses the provided
// helper functions to realize it. Those functions are only
// visible inside the state package.
type AgentMixinEntity struct {
	root  string
	key   string
	mixin *agentMixin
}

func NewAgentMixinEntity(st *State, root, key string) *AgentMixinEntity {
	ame := &AgentMixinEntity{root, key, nil}
	ame.mixin = newAgentMixin(st, ame.zkAgentPath())
	return ame
}

func (ame *AgentMixinEntity) Key() string {
	return ame.key
}

func (ame *AgentMixinEntity) zkAgentPath() string {
	return fmt.Sprintf("/%s/%s/agent", ame.root, ame.key)
}

func (ame *AgentMixinEntity) AgentConnected() (bool, error) {
	return ame.mixin.connected()
}

func (ame *AgentMixinEntity) WaitAgentConnected() error {
	return ame.mixin.waitConnected()
}

func (ame *AgentMixinEntity) ConnectAgent() error {
	return ame.mixin.connect()
}

func (ame *AgentMixinEntity) DisconnectAgent() error {
	return ame.mixin.disconnect()
}

var _ = AgentMixin(&AgentMixinEntity{})
