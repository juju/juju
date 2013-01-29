package api

import (
	"code.google.com/p/go.net/websocket"
	"launchpad.net/juju-core/state"
)

// srvRoot represents a single client's connection to the state.
type srvRoot struct {
	admin *srvAdmin
	srv   *Server
	conn  *websocket.Conn
}

type srvAdmin struct {
	mu     sync.Mutex
	root   *srvRoot
	entity string
}

type srvMachine struct {
	m *state.Machine
}

type rpcId struct {
	Id string
}

func newStateServer(srv *Server, conn *websocket.Conn) *srvRoot {
	r := &srvRoot{
		srv:  srv,
		conn: conn,
	}
	r.admin = &srvAdmin{
		root: r,
	}
	return r
}

var (
	errBadId       = errors.New("id not found")
	errBadCreds    = errors.New("invalid entity name or password")
	errNotLoggedIn = errors.New("not logged in")
)

func (st *srvRoot) Admin(id string) (*srvAdmin, error) {
	if id != "" {
		return nil, errBadId
	}
	return st.admin
}

type rpcCreds struct {
	EntityName string
	Password   string
}

func (a *srvAdmin) Login(c rpcCreds) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	entity, err := a.srv.state.Entity(c.EntityName)
	if err != nil && err != state.IsNotFound {
		return err
	}
	// We return the same error when an entity
	// does not exist as for a bad password, so that
	// we don't allow unauthenticated users to find information
	// about existing entities.
	if err != nil || !entity.PasswordValid(c.Password) {
		return errBadCreds
	}
	a.entity = c.EntityName
	return nil
}

func (a *srvAdmin) loggedIn() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.entity != ""
}

func (st *srvRoot) Machine(id string) (*srvMachine, error) {
	if !st.a.loggedIn() {
		return nil, errNotLoggedIn
	}
	m, err := st.srv.state.Machine(id)
	if err != nil {
		return nil, err
	}
	return &srvMachine{m}, nil
}

func (m *srvMachine) InstanceId() (rpcId, error) {
	id, err := m.m.InstanceId()
	return rpcId{string(id)}, err
}
