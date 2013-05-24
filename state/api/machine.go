// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state/api/params"
)

// Machine represents the state of a machine.
type Machine struct {
	st  *State
	id  string
	doc params.Machine
}

// MachineInfo holds information about a machine.
type MachineInfo struct {
	InstanceId string // blank if not set.
}

// Tag returns a name identifying the machine that is safe to use
// as a file name.  The returned name will be different from other
// Tag values returned by any other entities from the same state.
func (m *Machine) Tag() string {
	return MachineTag(m.Id())
}

// MachineTag returns the tag for the
// machine with the given id.
func MachineTag(id string) string {
	return fmt.Sprintf("machine-%s", id)
}

// Id returns the machine id.
func (m *Machine) Id() string {
	return m.id
}

// SetPassword sets the password for the machine's agent.
func (m *Machine) SetPassword(password string) error {
	return m.st.call("Machine", m.id, "SetPassword", &params.Password{
		Password: password,
	}, nil)
}

func (m *Machine) Watch() *EntityWatcher {
	return newEntityWatcher(m.st, "Machine", m.id)
}

// Refresh refreshes the contents of the machine from the underlying
// state. TODO(rog) It returns a NotFoundError if the machine has been removed.
func (m *Machine) Refresh() error {
	return m.st.call("Machine", m.id, "Get", nil, &m.doc)
}

// String returns the machine's id.
func (m *Machine) String() string {
	return m.id
}

// InstanceId returns the provider specific instance id for this machine
// and whether it has been set.
func (m *Machine) InstanceId() (string, bool) {
	return m.doc.InstanceId, m.doc.InstanceId != ""
}

// SetAgentAlive signals that the agent for machine m is alive. It
// returns the started pinger.
func (m *Machine) SetAgentAlive() (*Pinger, error) {
	var id params.PingerId
	err := m.st.call("Machine", m.id, "SetAgentAlive", nil, &id)
	if err != nil {
		return nil, err
	}
	return &Pinger{
		st: m.st,
		id: id.PingerId,
	}, nil
}

// EnsureDead sets the machine lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise. EnsureDead will fail if the machine has
// principal units assigned, or if the machine has JobManageEnviron.
// If the machine has assigned units, EnsureDead will return
// a CodeHasAssignedUnits error.
func (m *Machine) EnsureDead() error {
	return m.st.call("Machine", m.id, "EnsureDead", nil, nil)
}

// Remove removes the machine from state. It will fail if the machine
// is not Dead.
func (m *Machine) Remove() error {
	return m.st.call("Machine", m.id, "Remove", nil, nil)
}

// Constraints returns the exact constraints that should apply when
// provisioning an instance for the machine.
func (m *Machine) Constraints() (constraints.Value, error) {
	var results params.ConstraintsResults
	err := m.st.call("Machine", m.id, "Constraints", nil, &results)
	if err != nil {
		return constraints.Value{}, err
	}
	return results.Constraints, nil
}

// SetProvisioned sets the provider specific machine id and nonce for
// this machine. Once set, the instance id cannot be changed.
func (m *Machine) SetProvisioned(id, nonce string) error {
	return m.st.call("Machine", m.id, "SetProvisioned", &params.SetProvisioned{
		InstanceId: id,
		Nonce:      nonce,
	}, nil)
}

// Status returns the status of the machine.
func (m *Machine) Status() (params.Status, string, error) {
	var results params.StatusResults
	err := m.st.call("Machine", m.id, "Status", nil, &results)
	if err != nil {
		return "", "", err
	}
	return results.Status, results.Info, nil
}

// SetStatus sets the status of the machine.
func (m *Machine) SetStatus(status params.Status, info string) error {
	return m.st.call("Machine", m.id, "SetStatus", &params.SetStatus{
		Status: status,
		Info:   info,
	}, nil)
}

// Life returns whether the machine is "alive", "dying" or "dead".
func (m *Machine) Life() params.Life {
	return m.doc.Life
}

// Series returns the operating system series running on the machine.
func (m *Machine) Series() string {
	return m.doc.Series
}
