package api

import (
	"code.google.com/p/go.net/websocket"
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/statecmd"
	statewatcher "launchpad.net/juju-core/state/watcher"
	"strconv"
	"sync"
)

// TODO(rog) remove this when the rest of the system
// has been updated to set passwords appropriately.
var AuthenticationEnabled = false

// srvRoot represents a single client's connection to the state.
type srvRoot struct {
	admin    *srvAdmin
	client   *srvClient
	srv      *Server
	conn     *websocket.Conn
	watchers *watchers

	user authUser
}

// srvAdmin is the only object that unlogged-in
// clients can access. It holds any methods
// that are needed to log in.
type srvAdmin struct {
	root *srvRoot
}

// srvMachine serves API methods on a machine.
type srvMachine struct {
	root *srvRoot
	m    *state.Machine
}

// srvUnit serves API methods on a unit.
type srvUnit struct {
	root *srvRoot
	u    *state.Unit
}

// srvUser serves API methods on a state User.
type srvUser struct {
	root *srvRoot
	u    *state.User
}

// srvClient serves client-specific API methods.
type srvClient struct {
	root *srvRoot
}

func newStateServer(srv *Server, conn *websocket.Conn) *srvRoot {
	r := &srvRoot{
		srv:      srv,
		conn:     conn,
		watchers: newWatchers(),
	}
	r.admin = &srvAdmin{
		root: r,
	}
	r.client = &srvClient{
		root: r,
	}
	return r
}

// Kill implements rpc.Killer.  It cleans up any resources that need
// cleaning up to ensure that all outstanding requests return.
func (r *srvRoot) Kill() {
	r.watchers.stopAll()
}

// Admin returns an object that provides API access
// to methods that can be called even when not
// authenticated.
func (r *srvRoot) Admin(id string) (*srvAdmin, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, errBadId
	}
	return r.admin, nil
}

// requireAgent checks whether the current client is an agent and hence
// may access the agent APIs.  We filter out non-agents when calling one
// of the accessor functions (Machine, Unit, etc) which avoids us making
// the check in every single request method.
func (r *srvRoot) requireAgent() error {
	e := r.user.entity()
	if e == nil {
		return errNotLoggedIn
	}
	if !isAgent(e) {
		return errPerm
	}
	return nil
}

// requireClient returns an error unless the current
// client is a juju client user.
func (r *srvRoot) requireClient() error {
	e := r.user.entity()
	if e == nil {
		return errNotLoggedIn
	}
	if isAgent(e) {
		return errPerm
	}
	return nil
}

// Machine returns an object that provides
// API access to methods on a state.Machine.
func (r *srvRoot) Machine(id string) (*srvMachine, error) {
	if err := r.requireAgent(); err != nil {
		return nil, err
	}
	m, err := r.srv.state.Machine(id)
	if err != nil {
		return nil, err
	}
	return &srvMachine{
		root: r,
		m:    m,
	}, nil
}

// Unit returns an object that provides
// API access to methods on a state.Unit.
func (r *srvRoot) Unit(name string) (*srvUnit, error) {
	if err := r.requireAgent(); err != nil {
		return nil, err
	}
	u, err := r.srv.state.Unit(name)
	if err != nil {
		return nil, err
	}
	return &srvUnit{
		root: r,
		u:    u,
	}, nil
}

// User returns an object that provides
// API access to methods on a state.User.
func (r *srvRoot) User(name string) (*srvUser, error) {
	// Any user is allowed to access their own user object.
	// We check at this level rather than at the operation
	// level to stop malicious probing for current user names.
	// When we provide support for user administration,
	// this will need to be changed to allow access to
	// the administrator.
	e := r.user.entity()
	if e == nil {
		return nil, errNotLoggedIn
	}
	if e.EntityName() != name {
		return nil, errPerm
	}
	u, err := r.srv.state.User(name)
	if err != nil {
		return nil, err
	}
	return &srvUser{
		root: r,
		u:    u,
	}, nil
}

// EntityWatcher returns an object that provides
// API access to methods on a state.EntityWatcher.
// Each client has its own current set of watchers, stored
// in r.watchers.
func (r *srvRoot) EntityWatcher(id string) (srvEntityWatcher, error) {
	if err := r.requireAgent(); err != nil {
		return srvEntityWatcher{}, err
	}
	w := r.watchers.get(id)
	if w == nil {
		return srvEntityWatcher{}, errUnknownWatcher
	}
	if _, ok := w.w.(*state.EntityWatcher); !ok {
		return srvEntityWatcher{}, errUnknownWatcher
	}
	return srvEntityWatcher{w}, nil
}

// Client returns an object that provides access
// to methods accessible to non-agent clients.
func (r *srvRoot) Client(id string) (*srvClient, error) {
	if err := r.requireClient(); err != nil {
		return nil, err
	}
	if id != "" {
		// Safeguard id for possible future use.
		return nil, errBadId
	}
	return r.client, nil
}

type srvEntityWatcher struct {
	*srvWatcher
}

// Next returns when a change has occurred to the
// entity being watched since the most recent call to Next
// or the Watch call that created the EntityWatcher.
func (w srvEntityWatcher) Next() error {
	if _, ok := <-w.w.(*state.EntityWatcher).Changes(); ok {
		return nil
	}
	err := w.w.Err()
	if err == nil {
		err = errStoppedWatcher
	}
	return err
}

func (c *srvClient) Status() (Status, error) {
	ms, err := c.root.srv.state.AllMachines()
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Machines: make(map[string]MachineInfo),
	}
	for _, m := range ms {
		instId, _ := m.InstanceId()
		status.Machines[m.Id()] = MachineInfo{
			InstanceId: string(instId),
		}
	}
	return status, nil
}

// ServiceSet implements the server side of Client.ServerSet.
func (c *srvClient) ServiceSet(p statecmd.ServiceSetParams) error {
	return statecmd.ServiceSet(c.root.srv.state, p)
}

// ServiceSetYAML implements the server side of Client.ServerSetYAML.
func (c *srvClient) ServiceSetYAML(p statecmd.ServiceSetYAMLParams) error {
	return statecmd.ServiceSetYAML(c.root.srv.state, p)
}

// ServiceGet returns the configuration for a service.
func (c *srvClient) ServiceGet(args statecmd.ServiceGetParams) (statecmd.ServiceGetResults, error) {
	return statecmd.ServiceGet(c.root.srv.state, args)
}

// ServiceExpose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *srvClient) ServiceExpose(args statecmd.ServiceExposeParams) error {
	return statecmd.ServiceExpose(c.root.srv.state, args)
}

// ServiceUnexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *srvClient) ServiceUnexpose(args statecmd.ServiceUnexposeParams) error {
	return statecmd.ServiceUnexpose(c.root.srv.state, args)
}

// CharmInfo returns information about the requested charm.
func (c *srvClient) CharmInfo(args CharmInfoParams) (CharmInfo, error) {
	curl, err := charm.ParseURL(args.CharmURL)
	if err != nil {
		return CharmInfo{}, err
	}
	charm, err := c.root.srv.state.Charm(curl)
	if err != nil {
		return CharmInfo{}, err
	}
	meta := charm.Meta()
	info := CharmInfo{
		Name:        meta.Name,
		Revision:    charm.Revision(),
		Subordinate: meta.Subordinate,
		URL:         curl.String(),
	}
	return info, nil
}

// EnvironmentInfo returns information about the current environment (default
// series and type).
func (c *srvClient) EnvironmentInfo() (EnvironmentInfo, error) {
	conf, err := c.root.srv.state.EnvironConfig()
	if err != nil {
		return EnvironmentInfo{}, err
	}
	info := EnvironmentInfo{
		DefaultSeries: conf.DefaultSeries(),
		ProviderType:  conf.Type(),
	}
	return info, nil
}

type rpcCreds struct {
	EntityName string
	Password   string
}

// Login logs in with the provided credentials.
// All subsequent requests on the connection will
// act as the authenticated user.
func (a *srvAdmin) Login(c rpcCreds) error {
	return a.root.user.login(a.root.srv.state, c.EntityName, c.Password)
}

type rpcMachine struct {
	InstanceId string
}

// Get retrieves all the details of a machine.
func (m *srvMachine) Get() (info rpcMachine) {
	instId, _ := m.m.InstanceId()
	info.InstanceId = string(instId)
	return
}

type rpcEntityWatcherId struct {
	EntityWatcherId string
}

func (m *srvMachine) Watch() (rpcEntityWatcherId, error) {
	w := m.m.Watch()
	if _, ok := <-w.Changes(); !ok {
		return rpcEntityWatcherId{}, statewatcher.MustErr(w)
	}
	return rpcEntityWatcherId{
		EntityWatcherId: m.root.watchers.register(w).id,
	}, nil
}

type rpcPassword struct {
	Password string
}

func setPassword(e state.AuthEntity, password string) error {
	// Catch expected common case of mispelled
	// or missing Password parameter.
	if password == "" {
		return fmt.Errorf("password is empty")
	}
	return e.SetPassword(password)
}

// SetPassword sets the machine's password.
func (m *srvMachine) SetPassword(p rpcPassword) error {
	// Allow:
	// - the machine itself.
	// - the environment manager.
	e := m.root.user.entity()
	allow := e.EntityName() == m.m.EntityName() ||
		isMachineWithJob(e, state.JobManageEnviron)
	if !allow {
		return errPerm
	}
	return setPassword(m.m, p.Password)
}

// Get retrieves all the details of a unit.
func (u *srvUnit) Get() (rpcUnit, error) {
	var ru rpcUnit
	ru.DeployerName, _ = u.u.DeployerName()
	// TODO add other unit attributes
	return ru, nil
}

// SetPassword sets the unit's password.
func (u *srvUnit) SetPassword(p rpcPassword) error {
	ename := u.root.user.entity().EntityName()
	// Allow:
	// - the unit itself.
	// - the machine responsible for unit, if unit is principal
	// - the unit's principal unit, if unit is subordinate
	allow := ename == u.u.EntityName()
	if !allow {
		deployerName, ok := u.u.DeployerName()
		allow = ok && ename == deployerName
	}
	if !allow {
		return errPerm
	}
	return setPassword(u.u, p.Password)
}

type rpcUnit struct {
	DeployerName string
	// TODO(rog) other unit attributes.
}

// SetPassword sets the user's password.
func (u *srvUser) SetPassword(p rpcPassword) error {
	return setPassword(u.u, p.Password)
}

type rpcUser struct {
	// This is a placeholder for any information
	// that may be associated with a user in the
	// future.
}

// Get retrieves all details of a user.
func (u *srvUser) Get() (rpcUser, error) {
	return rpcUser{}, nil
}

// authUser holds login details. It's ok to call
// its methods concurrently.
type authUser struct {
	mu      sync.Mutex
	_entity state.AuthEntity // logged-in entity (access only when mu is locked)
}

// login authenticates as entity with the given name,.
func (u *authUser) login(st *state.State, entityName, password string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	entity, err := st.AuthEntity(entityName)
	if err != nil && !state.IsNotFound(err) {
		return err
	}
	// TODO(rog) remove
	if !AuthenticationEnabled {
		u._entity = entity
		return nil
	}
	// We return the same error when an entity
	// does not exist as for a bad password, so that
	// we don't allow unauthenticated users to find information
	// about existing entities.
	if err != nil || !entity.PasswordValid(password) {
		return errBadCreds
	}
	u._entity = entity
	return nil
}

// entity returns the currently logged-in entity, or nil if not
// currently logged on.  The returned entity should not be modified
// because it may be used concurrently.
func (u *authUser) entity() state.AuthEntity {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u._entity
}

// isMachineWithJob returns whether the given entity is a machine that
// is configured to run the given job.
func isMachineWithJob(e state.AuthEntity, j state.MachineJob) bool {
	m, ok := e.(*state.Machine)
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

// isAgent returns whether the given entity is an agent.
func isAgent(e state.AuthEntity) bool {
	_, isUser := e.(*state.User)
	return !isUser
}

// watcher represents the interface provided by state watchers.
type watcher interface {
	Stop() error
	Err() error
}

// watchers holds all the watchers for a connection.
type watchers struct {
	mu    sync.Mutex
	maxId uint64
	ws    map[string]*srvWatcher
}

// srvWatcher holds the details of a watcher.  It also implements the
// Stop RPC method for all watchers.
type srvWatcher struct {
	ws *watchers
	w  watcher
	id string
}

// Stop stops the given watcher. It causes any outstanding
// Next calls to return a CodeStopped error.
// Any subsequent Next calls will return a CodeNotFound
// error because the watcher will no longer exist.
func (w *srvWatcher) Stop() error {
	err := w.w.Stop()
	w.ws.mu.Lock()
	defer w.ws.mu.Unlock()
	delete(w.ws.ws, w.id)
	return err
}

func newWatchers() *watchers {
	return &watchers{
		ws: make(map[string]*srvWatcher),
	}
}

// get returns the srvWatcher registered with the given
// id, or nil if there is no such watcher.
func (ws *watchers) get(id string) *srvWatcher {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.ws[id]
}

// register records the given watcher and returns
// a srvWatcher instance for it.
func (ws *watchers) register(w watcher) *srvWatcher {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.maxId++
	sw := &srvWatcher{
		ws: ws,
		id: strconv.FormatUint(ws.maxId, 10),
		w:  w,
	}
	ws.ws[sw.id] = sw
	return sw
}

func (ws *watchers) stopAll() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	for _, w := range ws.ws {
		if err := w.w.Stop(); err != nil {
			log.Printf("state/api: error stopping %T watcher: %v", w, err)
		}
	}
	ws.ws = make(map[string]*srvWatcher)
}
