package state

import (
	"fmt"
	"launchpad.net/juju-core/state/presence"
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
	agentVersion
}

func newMachine(st *State, key string) *Machine {
	m := &Machine{
		st:  st,
		key: key,
	}
	m.agentVersion = agentVersion{
		zk:    st.zk,
		agent: "machine",
		path:  m.zkPath(),
	}
	return m
}

// Id returns the machine id.
func (m *Machine) Id() int {
	return keySeq(m.key)
}

// AgentAlive returns whether the respective remote agent is alive.
func (m *Machine) AgentAlive() (bool, error) {
	return presence.Alive(m.st.zk, m.zkAgentPath())
}

// WaitAgentAlive blocks until the respective agent is alive.
func (m *Machine) WaitAgentAlive(timeout time.Duration) error {
	err := presence.WaitAlive(m.st.zk, m.zkAgentPath(), timeout)
	if err != nil {
		return fmt.Errorf("waiting for agent of machine %s: %v", m, err)
	}
	return nil
}

// SetAgentAlive signals that the agent for machine m is alive
// by starting a pinger on its presence node. It returns the
// started pinger.
func (m *Machine) SetAgentAlive() (*presence.Pinger, error) {
	return presence.StartPinger(m.st.zk, m.zkAgentPath(), agentPingerPeriod)
}

// SetInstanceId sets the provider specific machine id for this machine.
func (m *Machine) SetInstanceId(id string) (err error) {
	return setConfigString(m.st.zk, m.zkPath(), providerMachineId, id,
		"instance id of machine %v", m.String())
}

// InstanceId returns the provider specific machine id for this machine.
// If the id is not set, or its value is "" an error of type NotFoundError
// will be returned.
func (m *Machine) InstanceId() (string, error) {
	instanceId, err := getConfigString(m.st.zk, m.zkPath(), providerMachineId,
		"instance id of machine %v", m.String())
	if _, ok := err.(*NotFoundError); ok || (err == nil && instanceId == "") {
		return "", &NotFoundError{
			fmt.Sprintf("instance id for machine %d is not set", m.Id()),
		}
	}
	return instanceId, err
}

// Units returns all the units that have been assigned
// to the machine.
func (m *Machine) Units() (units []*Unit, err error) {
	defer errorContextf(&err, "cannot get all assigned units of machine %s", m)
	topology, err := readTopology(m.st.zk)
	if err != nil {
		return nil, err
	}
	keys := topology.UnitsForMachine(m.key)
	units = make([]*Unit, len(keys))
	for i, key := range keys {
		units[i], err = m.st.unitFromKey(topology, key)
		if err != nil {
			return nil, fmt.Errorf("inconsistent topology: %v", err)
		}
	}
	return units, nil
}

func (m *Machine) WatchUnits() *MachineUnitsWatcher {
	return newMachineUnitsWatcher(m)
}

func (m *Machine) Watch() *MachineWatcher {
	return newMachineWatcher(m)
}

// String returns a unique description of this machine
func (m *Machine) String() string {
	return strconv.Itoa(m.Id())
}

// zkPath returns the ZooKeeper base path for the machine.
func (m *Machine) zkPath() string {
	return path.Join(zkMachinesPath, m.key)
}

// zkAgentPath returns the ZooKeeper path for the machine agent.
func (m *Machine) zkAgentPath() string {
	return path.Join(m.zkPath(), "agent")
}

// keySeq returns the sequence number part of
// the the given machine or unit key.
func keySeq(key string) (id int) {
	if key == "" {
		panic("keySeq: empty key")
	}
	i := strings.LastIndex(key, "-")
	var id64 int64
	var err error
	if i >= 0 {
		id64, err = strconv.ParseInt(key[i+1:], 10, 32)
	}
	if i < 0 || err != nil {
		panic("keySeq: invalid key: " + key)
	}
	return int(id64)
}

// machineKey returns the machine key corresponding to machineId.
func machineKey(machineId int) string {
	return fmt.Sprintf("machine-%010d", machineId)
}
