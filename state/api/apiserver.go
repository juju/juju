package api

import (
	"code.google.com/p/go.net/websocket"
	"launchpad.net/juju-core/state"
)

// srvState represents a single client's connection to the state.
type srvState struct {
	srv  *Server
	conn *websocket.Conn
}

func newStateServer(srv *Server, conn *websocket.Conn) *srvState {
	return &srvState{
		srv:  srv,
		conn: conn,
	}
}

type rpcId struct {
	Id string
}

func (st *srvState) Machine(id string) (*srvMachine, error) {
	m, err := st.srv.state.Machine(id)
	if err != nil {
		return nil, err
	}
	return &srvMachine{m}, nil
}

type srvMachine struct {
	m *state.Machine
}

type rpcMachine struct {
	InstanceId string
}

func (m *srvMachine) Get() (info rpcMachine) {
	instId, _ := m.m.InstanceId()
	info.InstanceId = string(instId)
	return
}
