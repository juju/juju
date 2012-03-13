// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"strconv"
	"strings"
)

// Machine represents the state of a machine.
type Machine struct {
	zk  *zookeeper.Conn
	key string
}

// Id returns the machine id.
func (m *Machine) Id() int {
	return machineId(m.key)
}

// zkKey returns the ZooKeeper key of the machine.
func (m *Machine) zkKey() string {
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

// machineId returns the machine id corresponding to machineKey.
func machineId(machineKey string) (id int) {
	if machineKey == "" {
		panic("machineId: empty machine key")
	}
	i := strings.Index(machineKey, "-")
	var id64 int64
	var err error
	if i >= 0 {
		id64, err = strconv.ParseInt(machineKey[i+1:], 10, 32)
	}
	if i < 0 || err != nil {
		panic("machineId: invalid machine key: " + machineKey)
	}
	return int(id64)
}

// machineKey returns the machine key corresponding to machineId.
func machineKey(machineId int) string {
	return fmt.Sprintf("machine-%010d", machineId)
}
