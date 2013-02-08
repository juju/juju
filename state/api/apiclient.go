package api

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/rpc"
	"strings"
)

// Machine represents the state of a machine.
type Machine struct {
	st  *State
	id  string
	doc rpcMachine
}

// Machine returns a reference to the machine with the given id.
func (st *State) Machine(id string) (*Machine, error) {
	m := &Machine{
		st: st,
		id: id,
	}
	if err := m.Refresh(); err != nil {
		return nil, err
	}
	return m, nil
}

// Unit represents the state of a service unit.
type Unit struct {
	st   *State
	name string
	doc  rpcUnit
}

// Unit returns a unit by name.
func (st *State) Unit(name string) (*Unit, error) {
	u := &Unit{
		st:   st,
		name: name,
	}
	if err := u.Refresh(); err != nil {
		return nil, err
	}
	return u, nil
}

// Login authenticates as the entity with the given name and password.
// Subsequent requests on the state will act as that entity.
// This method is usually called automatically by Open.
func (st *State) Login(entityName, password string) error {
	err := st.client.Call("Admin", "", "Login", &rpcCreds{
		EntityName: entityName,
		Password:   password,
	}, nil)
	return rpcError(err)
}

// Id returns the machine id.
func (m *Machine) Id() string {
	return m.id
}

// EntityName returns a name identifying the machine that is safe to use
// as a file name.  The returned name will be different from other
// EntityName values returned by any other entities from the same state.
func (m *Machine) EntityName() string {
	return MachineEntityName(m.Id())
}

// MachineEntityName returns the entity name for the
// machine with the given id.
func MachineEntityName(id string) string {
	return fmt.Sprintf("machine-%s", id)
}

// Refresh refreshes the contents of the machine from the underlying
// state. TODO(rog) It returns a NotFoundError if the machine has been removed.
func (m *Machine) Refresh() error {
	err := m.st.client.Call("Machine", m.id, "Get", nil, &m.doc)
	return rpcError(err)
}

// String returns the machine's id.
func (m *Machine) String() string {
	return m.id
}

// InstanceId returns the provider specific instance id for this machine.
func (m *Machine) InstanceId() (string, error) {
	if m.doc.InstanceId == "" {
		return "", fmt.Errorf("instance id for machine %v not found", m.id)
	}
	return m.doc.InstanceId, nil
}

// SetPassword sets the password for the machine's agent.
func (m *Machine) SetPassword(password string) error {
	err := m.st.client.Call("Machine", m.id, "SetPassword", &rpcPassword{
		Password: password,
	}, nil)
	return rpcError(err)

}

// Refresh refreshes the contents of the Unit from the underlying
// state. TODO(rog) It returns a NotFoundError if the unit has been removed.
func (u *Unit) Refresh() error {
	err := u.st.client.Call("Unit", u.name, "Get", nil, &u.doc)
	return rpcError(err)
}

// SetPassword sets the password for the unit's agent.
func (u *Unit) SetPassword(password string) error {
	err := u.st.client.Call("Unit", u.name, "SetPassword", &rpcPassword{
		Password: password,
	}, nil)
	return rpcError(err)
}

// UnitEntityName returns the entity name for the
// unit with the given name.
func UnitEntityName(unitName string) string {
	return "unit-" + strings.Replace(unitName, "/", "-", -1)
}

// EntityName returns a name identifying the unit that is safe to use
// as a file name.  The returned name will be different from other
// EntityName values returned by any other entities from the same state.
func (u *Unit) EntityName() string {
	return UnitEntityName(u.name)
}

// DeployerName returns the entity name of the agent responsible for deploying
// the unit. If no such entity can be determined, false is returned.
func (u *Unit) DeployerName() (string, bool) {
	return u.doc.DeployerName, u.doc.DeployerName != ""
}

// rpcError maps errors returned from an RPC call into local errors with
// appropriate values.
// TODO(rog): implement NotFoundError, etc.
func rpcError(err error) error {
	if err == nil {
		return nil
	}
	rerr, ok := err.(*rpc.RemoteError)
	if !ok {
		return err
	}
	// TODO(rog) map errors into known error types, possibly introducing
	// error codes to do so.
	return errors.New(rerr.Message)
}
