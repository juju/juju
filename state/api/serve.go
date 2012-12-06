package api

import (
	"fmt"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state"
	"net"
	"net/http"
)

// srvState represents a single client's connection to the state.
type srvState struct {
	state    *state.State
	c *websocket.Conn
}

// NewServer returns an http handler that serves the API
// interface to the given state as a websocket.
func NewServer(s *state.State) http.Handler {
	return websocket.Handler(func(c *websocket.Conn){
		ctxt.run(&srvState{
			state: s
			c: c,
		})
	})
}

type clientRequest struct {
	Seq uint64
	Name string
	Param interface{}
}

type serverRequest struct {
	Seq uint64
	Name string
	Param json.RawMessage
}

type clientReply struct {
	Seq uint64
	Result json.RawMessage
}

type serverReply struct {
	Seq uint64
	Error string
	Result interface{}
}

var rpcCalls = map[string] func(*srvState, *serverRequest) (interface{}, error) {
	srvState.Machine,
	srvState.AddMachine,
}

func (ctxt *serverConn) run() {
	var req serverRequest
	var reply serverReply
	for {
		req = serverRequest{}
		err := websocket.JSON.Receive(ctxt.c, &req)
		if err != nil {
			log.Printf("api: error receiving request: %v", err)
			return
		}
		reply = serverReply{Seq: req.Seq}
		if fn := rpcCalls[req.Name]; fn == nil {
			reply.Error = "name not known"
		} else {
			result, err := fn(srv, &req)
			if err != nil {
				reply.Error = err.Error()
			} else {
				reply.Result = result
			}
		}
		err = websocket.JSON.Send(ctxt.c, &reply)
		if err != nil {
			log.Printf("api: error sending reply: %v", err)
			return
		}
	}
}

type machineDoc struct {
	Id string
	InstanceId state.InstanceId
	Life state.Life
	Workers []state.WorkerKind
}

func newMachineDoc(m *state.Machine) *machineDoc {
	doc := &machineDoc{
		Id: m.Id(),
		Life: m.Life(),
		Workers: m.Workers(),
	}
	doc.InstanceId, _ = m.InstanceId()
	return doc
}

func (req *serverRequest) unmarshalParam(p interface{}) error {
	return json.Unmarshal([]byte(req.Param), p)
}

func (s *srvState) Machine(req *serverRequest) (interface{}, error) {
	var id string
	if err := req.unmarshalParam(&id); err != nil {
		return err
	}
	m, err := s.state.Machine(id)
	if err != nil {
		return nil, err
	}
	return newMachineDoc(m), nil
}

func (s *srvState) AllMachines() (interface{}, error) {
	sms, err := s.state.AllMachines()
	if err != nil {
		return nil, err
	}
	ms := make([]*machineDoc, len(sms))
	for i, m := range sms {
		ms[i] = newMachineDoc(m)
	}
	return ms, nil
}

func (s *srvState) AddMachine(req *serverRequest) (interface{}, error) {
	var workers []state.WorkerKind
	if err := req.unmarshalParam(&workers); err != nil {
		return err
	}
	m, err := s.state.AddMachine(workers...)
	if err != nil {
		return nil, err
	}
	return newMachineDoc(m), nil
}
