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

// Machine returns a reference to the machine with the given id.  It
// does not check whether the machine exists - subsequent requests will
// fail if it does not.
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
