// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/multiwatcher"
	"launchpad.net/juju-core/state/statecmd"
	statewatcher "launchpad.net/juju-core/state/watcher"
	"strconv"
	"sync"
)

// srvRoot represents a single client's connection to the state.
type srvRoot struct {
	admin    *srvAdmin
	client   *srvClient
	srv      *Server
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

func newStateServer(srv *Server) *srvRoot {
	r := &srvRoot{
		srv:      srv,
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
	e := r.user.authenticator()
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
	e := r.user.authenticator()
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
	e := r.user.authenticator()
	if e == nil {
		return nil, errNotLoggedIn
	}
	if e.Tag() != name {
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

func (r *srvRoot) AllWatcher(id string) (srvClientAllWatcher, error) {
	if err := r.requireClient(); err != nil {
		return srvClientAllWatcher{}, err
	}
	w := r.watchers.get(id)
	if w == nil {
		return srvClientAllWatcher{}, errUnknownWatcher
	}
	if _, ok := w.w.(*multiwatcher.Watcher); !ok {
		return srvClientAllWatcher{}, errUnknownWatcher
	}
	return srvClientAllWatcher{w}, nil

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
	ew := w.w.(*state.EntityWatcher)
	if _, ok := <-ew.Changes(); ok {
		return nil
	}
	err := ew.Err()
	if err == nil {
		err = errStoppedWatcher
	}
	return err
}

func (c *srvClient) Status() (api.Status, error) {
	ms, err := c.root.srv.state.AllMachines()
	if err != nil {
		return api.Status{}, err
	}
	status := api.Status{
		Machines: make(map[string]api.MachineInfo),
	}
	for _, m := range ms {
		instId, _ := m.InstanceId()
		status.Machines[m.Id()] = api.MachineInfo{
			InstanceId: string(instId),
		}
	}
	return status, nil
}

func (c *srvClient) WatchAll() (params.AllWatcherId, error) {
	w := c.root.srv.state.Watch()
	return params.AllWatcherId{
		AllWatcherId: c.root.watchers.register(w).id,
	}, nil
}

type srvClientAllWatcher struct {
	*srvWatcher
}

func (aw srvClientAllWatcher) Next() (params.AllWatcherNextResults, error) {
	deltas, err := aw.w.(*multiwatcher.Watcher).Next()
	return params.AllWatcherNextResults{
		Deltas: deltas,
	}, err
}

func (aw srvClientAllWatcher) Stop() error {
	return aw.w.(*multiwatcher.Watcher).Stop()
}

// ServiceSet implements the server side of Client.ServerSet.
func (c *srvClient) ServiceSet(p params.ServiceSet) error {
	svc, err := c.root.srv.state.Service(p.ServiceName)
	if err != nil {
		return err
	}
	return svc.SetConfig(p.Options)
}

// ServiceSetYAML implements the server side of Client.ServerSetYAML.
func (c *srvClient) ServiceSetYAML(p params.ServiceSetYAML) error {
	svc, err := c.root.srv.state.Service(p.ServiceName)
	if err != nil {
		return err
	}
	return svc.SetConfigYAML([]byte(p.Config))
}

// ServiceGet returns the configuration for a service.
func (c *srvClient) ServiceGet(args params.ServiceGet) (params.ServiceGetResults, error) {
	return statecmd.ServiceGet(c.root.srv.state, args)
}

// Resolved implements the server side of Client.Resolved.
func (c *srvClient) Resolved(p params.Resolved) error {
	unit, err := c.root.srv.state.Unit(p.UnitName)
	if err != nil {
		return err
	}
	return unit.Resolve(p.Retry)
}

// ServiceExpose changes the juju-managed firewall to expose any ports that
// were also explicitly marked by units as open.
func (c *srvClient) ServiceExpose(args params.ServiceExpose) error {
	return statecmd.ServiceExpose(c.root.srv.state, args)
}

// ServiceUnexpose changes the juju-managed firewall to unexpose any ports that
// were also explicitly marked by units as open.
func (c *srvClient) ServiceUnexpose(args params.ServiceUnexpose) error {
	return statecmd.ServiceUnexpose(c.root.srv.state, args)
}

var CharmStore charm.Repository = charm.Store

// ServiceDeploy fetches the charm from the charm store and deploys it.  Local
// charms are not supported.
func (c *srvClient) ServiceDeploy(args params.ServiceDeploy) error {
	state := c.root.srv.state
	conf, err := state.EnvironConfig()
	if err != nil {
		return err
	}
	curl, err := charm.InferURL(args.CharmUrl, conf.DefaultSeries())
	if err != nil {
		return err
	}
	conn, err := juju.NewConnFromState(state)
	if err != nil {
		return err
	}
	if args.NumUnits == 0 {
		args.NumUnits = 1
	}
	serviceName := args.ServiceName
	if serviceName == "" {
		serviceName = curl.Name
	}
	return statecmd.ServiceDeploy(state, args, conn, curl, CharmStore)
}

// AddServiceUnits adds a given number of units to a service.
func (c *srvClient) AddServiceUnits(args params.AddServiceUnits) (params.AddServiceUnitsResults, error) {
	units, err := statecmd.AddServiceUnits(c.root.srv.state, args)
	if err != nil {
		return params.AddServiceUnitsResults{}, err
	}
	unitNames := make([]string, len(units))
	for i, unit := range units {
		unitNames[i] = unit.String()
	}
	return params.AddServiceUnitsResults{Units: unitNames}, nil
}

// DestroyServiceUnits removes a given set of service units.
func (c *srvClient) DestroyServiceUnits(args params.DestroyServiceUnits) error {
	return statecmd.DestroyServiceUnits(c.root.srv.state, args)
}

// ServiceDestroy destroys a given service.
func (c *srvClient) ServiceDestroy(args params.ServiceDestroy) error {
	return statecmd.ServiceDestroy(c.root.srv.state, args)
}

// GetServiceConstraints returns the constraints for a given service.
func (c *srvClient) GetServiceConstraints(args params.GetServiceConstraints) (params.GetServiceConstraintsResults, error) {
	return statecmd.GetServiceConstraints(c.root.srv.state, args)
}

// SetServiceConstraints sets the constraints for a given service.
func (c *srvClient) SetServiceConstraints(args params.SetServiceConstraints) error {
	return statecmd.SetServiceConstraints(c.root.srv.state, args)
}

// AddRelation adds a relation between the specified endpoints and returns the relation info.
func (c *srvClient) AddRelation(args params.AddRelation) (params.AddRelationResults, error) {
	return statecmd.AddRelation(c.root.srv.state, args)
}

// DestroyRelation removes the relation between the specified endpoints.
func (c *srvClient) DestroyRelation(args params.DestroyRelation) error {
	return statecmd.DestroyRelation(c.root.srv.state, args)
}

// CharmInfo returns information about the requested charm.
func (c *srvClient) CharmInfo(args params.CharmInfo) (api.CharmInfo, error) {
	curl, err := charm.ParseURL(args.CharmURL)
	if err != nil {
		return api.CharmInfo{}, err
	}
	charm, err := c.root.srv.state.Charm(curl)
	if err != nil {
		return api.CharmInfo{}, err
	}
	info := api.CharmInfo{
		Revision: charm.Revision(),
		URL:      curl.String(),
		Config:   charm.Config(),
		Meta:     charm.Meta(),
	}
	return info, nil
}

// EnvironmentInfo returns information about the current environment (default
// series and type).
func (c *srvClient) EnvironmentInfo() (api.EnvironmentInfo, error) {
	conf, err := c.root.srv.state.EnvironConfig()
	if err != nil {
		return api.EnvironmentInfo{}, err
	}
	info := api.EnvironmentInfo{
		DefaultSeries: conf.DefaultSeries(),
		ProviderType:  conf.Type(),
		Name:          conf.Name(),
	}
	return info, nil
}

// GetAnnotations returns annotations about a given entity.
func (c *srvClient) GetAnnotations(args params.GetAnnotations) (params.GetAnnotationsResults, error) {
	entity, err := c.root.srv.state.Annotator(args.Tag)
	if err != nil {
		return params.GetAnnotationsResults{}, err
	}
	ann, err := entity.Annotations()
	if err != nil {
		return params.GetAnnotationsResults{}, err
	}
	return params.GetAnnotationsResults{Annotations: ann}, nil
}

// SetAnnotations stores annotations about a given entity.
func (c *srvClient) SetAnnotations(args params.SetAnnotations) error {
	entity, err := c.root.srv.state.Annotator(args.Tag)
	if err != nil {
		return err
	}
	return entity.SetAnnotations(args.Pairs)
}

// Login logs in with the provided credentials.
// All subsequent requests on the connection will
// act as the authenticated user.
func (a *srvAdmin) Login(c params.Creds) error {
	return a.root.user.login(a.root.srv.state, c.AuthTag, c.Password)
}

// Get retrieves all the details of a machine.
func (m *srvMachine) Get() (info params.Machine) {
	instId, _ := m.m.InstanceId()
	info.InstanceId = string(instId)
	return
}

func (m *srvMachine) Watch() (params.EntityWatcherId, error) {
	w := m.m.Watch()
	if _, ok := <-w.Changes(); !ok {
		return params.EntityWatcherId{}, statewatcher.MustErr(w)
	}
	return params.EntityWatcherId{
		EntityWatcherId: m.root.watchers.register(w).id,
	}, nil
}

func setPassword(e state.TaggedAuthenticator, password string) error {
	// Catch expected common case of mispelled
	// or missing Password parameter.
	if password == "" {
		return fmt.Errorf("password is empty")
	}
	return e.SetPassword(password)
}

// SetPassword sets the machine's password.
func (m *srvMachine) SetPassword(p params.Password) error {
	// Allow:
	// - the machine itself.
	// - the environment manager.
	e := m.root.user.authenticator()
	allow := e.Tag() == m.m.Tag() ||
		isMachineWithJob(e, state.JobManageEnviron)
	if !allow {
		return errPerm
	}
	return setPassword(m.m, p.Password)
}

// Get retrieves all the details of a unit.
func (u *srvUnit) Get() (params.Unit, error) {
	var ru params.Unit
	ru.DeployerTag, _ = u.u.DeployerTag()
	// TODO add other unit attributes
	return ru, nil
}

// SetPassword sets the unit's password.
func (u *srvUnit) SetPassword(p params.Password) error {
	tag := u.root.user.authenticator().Tag()
	// Allow:
	// - the unit itself.
	// - the machine responsible for unit, if unit is principal
	// - the unit's principal unit, if unit is subordinate
	allow := tag == u.u.Tag()
	if !allow {
		deployerTag, ok := u.u.DeployerTag()
		allow = ok && tag == deployerTag
	}
	if !allow {
		return errPerm
	}
	return setPassword(u.u, p.Password)
}

// SetPassword sets the user's password.
func (u *srvUser) SetPassword(p params.Password) error {
	return setPassword(u.u, p.Password)
}

// Get retrieves all details of a user.
func (u *srvUser) Get() (params.User, error) {
	return params.User{}, nil
}

// authUser holds login details. It's ok to call
// its methods concurrently.
type authUser struct {
	mu     sync.Mutex
	entity state.TaggedAuthenticator // logged-in entity (access only when mu is locked)
}

// login authenticates as entity with the given name,.
func (u *authUser) login(st *state.State, tag, password string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	entity, err := st.Authenticator(tag)
	if err != nil && !state.IsNotFound(err) {
		return err
	}
	// We return the same error when an entity
	// does not exist as for a bad password, so that
	// we don't allow unauthenticated users to find information
	// about existing entities.
	if err != nil || !entity.PasswordValid(password) {
		return errBadCreds
	}
	u.entity = entity
	return nil
}

// authenticator returns the currently logged-in authenticator entity, or nil
// if not currently logged on.  The returned entity should not be modified
// because it may be used concurrently.
func (u *authUser) authenticator() state.TaggedAuthenticator {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.entity
}

// isMachineWithJob returns whether the given entity is a machine that
// is configured to run the given job.
func isMachineWithJob(e state.TaggedAuthenticator, j state.MachineJob) bool {
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
func isAgent(e state.TaggedAuthenticator) bool {
	_, isUser := e.(*state.User)
	return !isUser
}

// watcher represents the interface provided by state watchers.
type watcher interface {
	Stop() error
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
			log.Errorf("state/api: error stopping %T watcher: %v", w, err)
		}
	}
	ws.ws = make(map[string]*srvWatcher)
}
