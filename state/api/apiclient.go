package api

import (
	"errors"
	"fmt"
	"launchpad.net/juju-core/rpc"
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

type Unit struct {
	st   *State
	name string
	doc  rpcUnit
}

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

func (m *Machine) Id() string {
	return m.id
}

func (m *Machine) EntityName() string {
	return "machine-" + m.Id()
}

func (m *Machine) Refresh() error {
	err := m.st.client.Call("Machine", m.id, "Get", nil, &m.doc)
	return rpcError(err)
}

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

func (u *Unit) Refresh() error {
	err := u.st.client.Call("Unit", u.name, "Get", nil, &u.doc)
	return rpcError(err)
}

func (u *Unit) DeployerName() (string, bool) {
	return u.doc.DeployerName, u.doc.DeployerName != ""
}

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
