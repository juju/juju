package api

import (
	"code.google.com/p/go.net/websocket"
	"launchpad.net/juju-core/state"
)

var (
	errBadId       = errors.New("id not found")
	errBadCreds    = errors.New("invalid entity name or password")
	errNotLoggedIn = errors.New("not logged in")
)

// srvRoot represents a single client's connection to the state.
type srvRoot struct {
	admin *srvAdmin
	srv   *Server
	conn  *websocket.Conn

	mu     sync.Mutex
	entity state.Entity		// logged-in entity
}

type srvAdmin struct {
	root   *srvRoot
}

type srvMachine struct {
	root *srvRoot
	m *state.Machine
}

type srvUnit struct {
	root *srvRoot
	u *state.Unit
}

type srvUser struct {
	root *srvRoot
	u *state.User
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

func (r *srvRoot) Admin(id string) (*srvAdmin, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, errBadId
	}
	return r.admin
}

func (r *srvRoot) Machine(id string) (*srvMachine, error) {
	if !r.a.loggedIn() {
		return nil, errNotLoggedIn
	}
	m, err := r.srv.state.Machine(id)
	if err != nil {
		return nil, err
	}
	return &srvMachine{m}, nil
}

func (r *srvRoot) Unit(name string) (*srvUnit, error) {
	if !r.a.loggedIn() {
		return nil, errNotLoggedIn
	}
	u, err := r.srv.state.Unit(name)
	return &srvUnit{u}, nil
}

func (r *srvRoot) User(name string) (*srvUser, error) {
	if !r.a.loggedIn() {
		return nil, errNotLoggedIn
	}
	u, err := r.srv.state.User(name)
	return &srvUser{u}, nil
}

type rpcCreds struct {
	EntityName string
	Password   string
}

func (a *srvAdmin) Login(c rpcCreds) error {
	a.root.mu.Lock()
	defer a.root.mu.Unlock()
	entity, err := a.root.state.Entity(c.EntityName)
	if err != nil && !state.IsNotFound(err) {
		return err
	}
	// We return the same error when an entity
	// does not exist as for a bad password, so that
	// we don't allow unauthenticated users to find information
	// about existing entities.
	if err != nil || !entity.PasswordValid(c.Password) {
		return errBadCreds
	}
	a.root.entity = c.EntityName
	return nil
}

type rpcPassword struct {
	Password string
}

func (r *srvRoot) loggedIn() bool {
	r.Lock()
	defer r.Unlock()
	return a.entity != nil
}

func (r *srvRoot) entityName() string {
	r.Lock()
	defer r.Unlock()
	return r.EntityName()
}

func (r *srvRoot) hasJob(j state.Job) bool {
	r.Lock()
	defer r.Unlock()
	m, ok := r.entity.(*state.Machine)
	if !ok {
		return false
	}
	for _, mj := range m.Jobs() {
		if mj == j {
			return true
		}
	}
	return false
}

type rpcMachine struct {
	InstanceId string
}

func (m *srvMachine) Get() (info rpcMachine) {
	instId, _ := m.m.InstanceId()
	info.InstanceId = string(instId)
	return
}

func (m *srvMachine) SetPassword(p rpcPassword) error {
	ename := m.root.a.entityName()
	// Allow:
	// - the machine itself.
	// - the environment manager.
	if m.root.entityName() != m.m.EntityName() &&
		m.root.hasJob(state.JobManageEnviron) {
		return errPerm
	}
	// Catch expected common case of mispelled
	// or missing Password parameter.
	if p.Password == "" {
		return fmt.Errorf("password is empty")
	}
	return m.m.SetPassword(p.Password)
}

func (u *srvUnit) SetPassword(p rpcPassword) error {
	allow unit itself
	machine responsible for unit, if principal
	principal unit, if subordinate
}