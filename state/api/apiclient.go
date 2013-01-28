package api

import (
	"errors"
	"launchpad.net/juju-core/rpc"
)

// Machine represents the state of a machine.
type Machine struct {
	st *State
	id string
}

// Machine returns a reference to the machine with the given id.  It
// does not check whether the machine exists - subsequent requests will
// fail if it does not.
func (st *State) Machine(id string) *Machine {
	return &Machine{
		st: st,
		id: id,
	}
}

// InstanceId returns the provider specific instance id for this machine.
func (m *Machine) InstanceId() (string, error) {
	var resp rpcId
	err := m.st.client.Call("Machine", m.id, "InstanceId", nil, &resp)
	if err != nil {
		return "", rpcError(err)
	}
	return resp.Id, nil
}

func rpcError(err error) error {
	rerr, ok := err.(*rpc.RemoteError)
	if !ok {
		return err
	}
	// TODO(rog) map errors into known error types, possibly introducing
	// error codes to do so.
	return errors.New(rerr.Message)
}
