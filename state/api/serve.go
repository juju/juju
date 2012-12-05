package api
import (
	"fmt"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/rpc"
	"net/http"
	"net"
)

type Server struct {
	srv *rpc.Server
	state *state.State
}

type context struct {
	state *state.State
	isHTTP bool
	hostAddr string
	// ... plus entity/authentication info.
}

func NewServer(s *state.State) *Server {
	rpcsrv, err := rpc.NewServer(&srvState{s})
	if err != nil {
		// This should only happen if srvState and friends are
		// malformed types.
		panic(err)	
	}
	return &Server{
		srv: rpcsrv,
		state: s,
	}
}

func (srv *Server) NewHTTPHandler() http.Handler {
	return srv.srv.NewHTTPHandler(func(req *http.Request) interface{} {
		return &context{
			state: srv.state,
			isHTTP: true,
			hostAddr: req.Host,
		}
	})
}

func (srv *Server) Accept(l net.Listener) error {
	return srv.srv.Accept(l, rpc.NewJSONServerCodec, func(c net.Conn) interface{} {
		return &context{
			state: srv.state,
			hostAddr: c.RemoteAddr().String(),
		}
	})
}

// srvState represents the server side of the API state - it
// is acted on by rpc.Server.
type srvState struct {
	state *state.State
}

func (s *srvState) CheckContext(ctxt *context) error {
	// TODO check if the given user is allowed any access at all.
	return nil
}

func (s *srvState) Machine(id string) (*srvMachine, error) {
	// TODO make this return a Machine without a round-trip
	// to the state.
	m, err := s.state.Machine(id)
	if err != nil {
		return nil, err
	}
	return &srvMachine{m, m.Id()}, nil
}

func (s *srvState) AllMachines() ([]*srvMachine, error) {
	sms, err := s.state.AllMachines()
	if err != nil {
		return nil, err
	}
	ms := make([]*srvMachine, len(sms))
	for i, m := range sms {
		ms[i] = &srvMachine{m, m.Id()}
	}
	return ms, nil
}

func (s *srvState) AddMachine(workers []state.WorkerKind) (*srvMachine, error) {
	m, err := s.state.AddMachine(workers...)
	if err != nil {
		return nil, err
	}
	return &srvMachine{m, m.Id()}, nil
}

func (s *srvState) EnvironConfig() (map[string] interface{}, error) {
	cfg, err := s.state.EnvironConfig()
	if err != nil {
		return nil, err
	}
	return cfg.AllAttrs(), nil
}

type srvMachine struct {
	m *state.Machine
	Id string
}

func (m *srvMachine) InstanceId() (string, error) {
	instId, err := m.m.InstanceId()
	return string(instId), err
}

func (m *srvMachine) SetInstanceId(ctxt *context, id state.InstanceId) error {
	if ctxt.isHTTP {
		// ... for example
		return fmt.Errorf("cannot set instance id from an http connection")
	}
	return m.m.SetInstanceId(id)
}
