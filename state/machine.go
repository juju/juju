// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"fmt"
	"launchpad.net/juju/go/state/presence"
	"path"
	"strconv"
	"strings"
	"time"
)

const providerMachineId = "provider-machine-id"

// Machine represents the state of a machine.
type Machine struct {
	st  *State
	key string
}

// Id returns the machine id.
func (m *Machine) Id() int {
	return machineId(m.key)
}

// AgentAlive returns whether the respective remote agent is alive.
func (m *Machine) AgentAlive() (bool, error) {
	return presence.Alive(m.st.zk, m.zkAgentPath())
}

// WaitAgentAlive blocks until the respective agent is alive.
func (m *Machine) WaitAgentAlive(timeout time.Duration) error {
	err := presence.WaitAlive(m.st.zk, m.zkAgentPath(), timeout)
	if err != nil {
		return fmt.Errorf("state: waiting for agent of machine %d: %v", m.Id(), err)
	}
	return nil
}

// SetAgentAlive signals that the agent for machine m is alive
// by starting a pinger on its presence node. It returns the
// started pinger.
func (m *Machine) SetAgentAlive() (*presence.Pinger, error) {
	return presence.StartPinger(m.st.zk, m.zkAgentPath(), agentPingerPeriod)
}

// InstanceId returns the provider-specific machine id for this machine.
func (m *Machine) InstanceId() (string, error) {
	config, err := readConfigNode(m.st.zk, m.zkPath())
	if err != nil {
		return "", err
	}
	v, ok := config.Get(providerMachineId)
	if !ok {
		// missing key is fine
		return "", nil
	}
	if id, ok := v.(string); ok {
		return id, nil
	}
	return "", fmt.Errorf("invalid contents, expecting string, got %T", v)
}

// SetInstanceId sets the provider-specific machine id for this machine.
func (m *Machine) SetInstanceId(id string) error {
	config, err := readConfigNode(m.st.zk, m.zkPath())
	if err != nil {
		return err
	}
	config.Set(providerMachineId, id)
	_, err = config.Write()
	return err
}

// zkKey returns the ZooKeeper key of the machine.
func (m *Machine) zkKey() string {
	return m.key
}

// zkPath returns the ZooKeeper base path for the machine.
func (m *Machine) zkPath() string {
	return path.Join(zkMachinesPath, m.zkKey())
}

// zkAgentPath returns the ZooKeeper path for the machine agent.
func (m *Machine) zkAgentPath() string {
	return path.Join(m.zkPath(), "agent")
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

// MachinesChange contains information about
// machines that have been added or deleted.
type MachinesChange struct {
	Added, Deleted []*Machine
}
