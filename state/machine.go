// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"strings"
)

// Machine represents the state of a machine.
type Machine struct {
	zk          *zookeeper.Conn
	key         string
}

// Key returns the public key of the machine.
func (m *Machine) Key() string {
	return publicMachineKey(m.key)
}

// InternalKey returns the internal key of the machine.
func (m *Machine) InternalKey() string {
	return m.key
}

// zkPath returns the ZooKeeper base path for the machine.
func (m *Machine) zkPath() string {
	return fmt.Sprintf("/machines/%s", m.key)
}

// zkAgentPath returns the ZooKeeper path for the machine agent.
func (m *Machine) zkAgentPath() string {
	return fmt.Sprintf("/machines/%s/agent", m.key)
}

// publicMachineKey returns the internal machine key 
// converted to an external one.
func publicMachineKey(internalKey string) string {
	if internalKey == "" {
		return ""
	}
	parts := strings.Split(internalKey, "-")
	sequence := parts[len(parts)-1]
	publicKey := strings.TrimLeft(sequence, "0")
	if len(publicKey) == 0 {
		// Key had only zeros, so machine 0.
		return "0"
	}
	return publicKey
}