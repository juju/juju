// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"errors"
	"time"

	"launchpad.net/tomb"

	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/apiserver/agent"
	"launchpad.net/juju-core/state/apiserver/charmrevisionupdater"
	"launchpad.net/juju-core/state/apiserver/client"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/apiserver/deployer"
	"launchpad.net/juju-core/state/apiserver/environment"
	"launchpad.net/juju-core/state/apiserver/firewaller"
	"launchpad.net/juju-core/state/apiserver/keymanager"
	"launchpad.net/juju-core/state/apiserver/keyupdater"
	loggerapi "launchpad.net/juju-core/state/apiserver/logger"
	"launchpad.net/juju-core/state/apiserver/machine"
	"launchpad.net/juju-core/state/apiserver/provisioner"
	"launchpad.net/juju-core/state/apiserver/rsyslog"
	"launchpad.net/juju-core/state/apiserver/uniter"
	"launchpad.net/juju-core/state/apiserver/upgrader"
	"launchpad.net/juju-core/state/apiserver/usermanager"
	"launchpad.net/juju-core/state/multiwatcher"
)

type clientAPI struct{ *client.API }

type taggedAuthenticator interface {
	state.Entity
	state.Authenticator
}

var (
	// maxClientPingInterval defines the timeframe until the ping timeout
	// closes the monitored connection. TODO(mue): Idea by Roger:
	// Move to API (e.g. params) so that the pinging there may
	// depend on the interval.
	maxClientPingInterval = 3 * time.Minute

	// mongoPingInterval defines the interval at which an API server
	// will ping the mongo session to make sure that it's still
	// alive. When the ping returns an error, the server will be
	// terminated.
	mongoPingInterval = 10 * time.Second
)

// srvRoot represents a single client's connection to the state
// after it has logged in.
type srvRoot struct {
	clientAPI
	srv       *Server
	rpcConn   *rpc.Conn
	resources *common.Resources

	entity taggedAuthenticator
}

// newSrvRoot creates the client's connection representation
// and starts a ping timeout for the monitoring of this
// connection.
func newSrvRoot(root *initialRoot, entity taggedAuthenticator) *srvRoot {
	r := &srvRoot{
		srv:       root.srv,
		rpcConn:   root.rpcConn,
		resources: common.NewResources(),
		entity:    entity,
	}
	r.resources.RegisterNamed("dataDir", common.StringResource(r.srv.dataDir))
	r.clientAPI.API = client.NewAPI(r.srv.state, r.resources, r)
	return r
}

// Kill implements rpc.Killer.  It cleans up any resources that need
// cleaning up to ensure that all outstanding requests return.
func (r *srvRoot) Kill() {
	r.resources.StopAll()
}

// requireAgent checks whether the current client is an agent and hence
// may access the agent APIs.  We filter out non-agents when calling one
// of the accessor functions (Machine, Unit, etc) which avoids us making
// the check in every single request method.
func (r *srvRoot) requireAgent() error {
	if !isAgent(r.entity) {
		return common.ErrPerm
	}
	return nil
}

// requireClient returns an error unless the current
// client is a juju client user.
func (r *srvRoot) requireClient() error {
	if isAgent(r.entity) {
		return common.ErrPerm
	}
	return nil
}

// KeyManager returns an object that provides access to the KeyManager API
// facade. The id argument is reserved for future use and currently
// needs to be empty.
func (r *srvRoot) KeyManager(id string) (*keymanager.KeyManagerAPI, error) {
	if id != "" {
		return nil, common.ErrBadId
	}
	return keymanager.NewKeyManagerAPI(r.srv.state, r.resources, r)
}

// UserManager returns an object that provides access to the UserManager API
// facade. The id argument is reserved for future use and currently
// needs to be empty
func (r *srvRoot) UserManager(id string) (*usermanager.UserManagerAPI, error) {
	if id != "" {
		return nil, common.ErrBadId
	}
	return usermanager.NewUserManagerAPI(r.srv.state, r)
}

// Machiner returns an object that provides access to the Machiner API
// facade. The id argument is reserved for future use and currently
// needs to be empty.
func (r *srvRoot) Machiner(id string) (*machine.MachinerAPI, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return machine.NewMachinerAPI(r.srv.state, r.resources, r)
}

// Provisioner returns an object that provides access to the
// Provisioner API facade. The id argument is reserved for future use
// and currently needs to be empty.
func (r *srvRoot) Provisioner(id string) (*provisioner.ProvisionerAPI, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return provisioner.NewProvisionerAPI(r.srv.state, r.resources, r)
}

// Uniter returns an object that provides access to the Uniter API
// facade. The id argument is reserved for future use and currently
// needs to be empty.
func (r *srvRoot) Uniter(id string) (*uniter.UniterAPI, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return uniter.NewUniterAPI(r.srv.state, r.resources, r)
}

// Firewaller returns an object that provides access to the Firewaller
// API facade. The id argument is reserved for future use and
// currently needs to be empty.
func (r *srvRoot) Firewaller(id string) (*firewaller.FirewallerAPI, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return firewaller.NewFirewallerAPI(r.srv.state, r.resources, r)
}

// Agent returns an object that provides access to the
// agent API.  The id argument is reserved for future use and must currently
// be empty.
func (r *srvRoot) Agent(id string) (*agent.API, error) {
	if id != "" {
		return nil, common.ErrBadId
	}
	return agent.NewAPI(r.srv.state, r)
}

// Deployer returns an object that provides access to the Deployer API facade.
// The id argument is reserved for future use and must be empty.
func (r *srvRoot) Deployer(id string) (*deployer.DeployerAPI, error) {
	if id != "" {
		// TODO(dimitern): There is no direct test for this
		return nil, common.ErrBadId
	}
	return deployer.NewDeployerAPI(r.srv.state, r.resources, r)
}

// Environment returns an object that provides access to the Environment API
// facade. The id argument is reserved for future use and currently needs to
// be empty.
func (r *srvRoot) Environment(id string) (*environment.EnvironmentAPI, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return environment.NewEnvironmentAPI(r.srv.state, r.resources, r)
}

// Rsyslog returns an object that provides access to the Rsyslog API
// facade. The id argument is reserved for future use and currently needs to
// be empty.
func (r *srvRoot) Rsyslog(id string) (*rsyslog.RsyslogAPI, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return rsyslog.NewRsyslogAPI(r.srv.state, r.resources, r)
}

// Logger returns an object that provides access to the Logger API facade.
// The id argument is reserved for future use and must be empty.
func (r *srvRoot) Logger(id string) (*loggerapi.LoggerAPI, error) {
	if id != "" {
		// TODO: There is no direct test for this
		return nil, common.ErrBadId
	}
	return loggerapi.NewLoggerAPI(r.srv.state, r.resources, r)
}

// Upgrader returns an object that provides access to the Upgrader API facade.
// The id argument is reserved for future use and must be empty.
func (r *srvRoot) Upgrader(id string) (upgrader.Upgrader, error) {
	if id != "" {
		// TODO: There is no direct test for this
		return nil, common.ErrBadId
	}
	// The type of upgrader we return depends on who is asking.
	// Machines get an UpgraderAPI, units get a UnitUpgraderAPI.
	// This is tested in the state/api/upgrader package since there
	// are currently no direct srvRoot tests.
	tagKind, _, err := names.ParseTag(r.GetAuthTag(), "")
	if err != nil {
		return nil, common.ErrPerm
	}
	switch tagKind {
	case names.MachineTagKind:
		return upgrader.NewUpgraderAPI(r.srv.state, r.resources, r)
	case names.UnitTagKind:
		return upgrader.NewUnitUpgraderAPI(r.srv.state, r.resources, r)
	}
	// Not a machine or unit.
	return nil, common.ErrPerm
}

// KeyUpdater returns an object that provides access to the KeyUpdater API facade.
// The id argument is reserved for future use and must be empty.
func (r *srvRoot) KeyUpdater(id string) (*keyupdater.KeyUpdaterAPI, error) {
	if id != "" {
		// TODO: There is no direct test for this
		return nil, common.ErrBadId
	}
	return keyupdater.NewKeyUpdaterAPI(r.srv.state, r.resources, r)
}

// CharmRevisionUpdater returns an object that provides access to the CharmRevisionUpdater API facade.
// The id argument is reserved for future use and must be empty.
func (r *srvRoot) CharmRevisionUpdater(id string) (*charmrevisionupdater.CharmRevisionUpdaterAPI, error) {
	if id != "" {
		// TODO: There is no direct test for this
		return nil, common.ErrBadId
	}
	return charmrevisionupdater.NewCharmRevisionUpdaterAPI(r.srv.state, r.resources, r)
}

// NotifyWatcher returns an object that provides
// API access to methods on a state.NotifyWatcher.
// Each client has its own current set of watchers, stored
// in r.resources.
func (r *srvRoot) NotifyWatcher(id string) (*srvNotifyWatcher, error) {
	if err := r.requireAgent(); err != nil {
		return nil, err
	}
	watcher, ok := r.resources.Get(id).(state.NotifyWatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvNotifyWatcher{
		watcher:   watcher,
		id:        id,
		resources: r.resources,
	}, nil
}

// StringsWatcher returns an object that provides API access to
// methods on a state.StringsWatcher.  Each client has its own
// current set of watchers, stored in r.resources.
func (r *srvRoot) StringsWatcher(id string) (*srvStringsWatcher, error) {
	if err := r.requireAgent(); err != nil {
		return nil, err
	}
	watcher, ok := r.resources.Get(id).(state.StringsWatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvStringsWatcher{
		watcher:   watcher,
		id:        id,
		resources: r.resources,
	}, nil
}

// RelationUnitsWatcher returns an object that provides API access to
// methods on a state.RelationUnitsWatcher. Each client has its own
// current set of watchers, stored in r.resources.
func (r *srvRoot) RelationUnitsWatcher(id string) (*srvRelationUnitsWatcher, error) {
	if err := r.requireAgent(); err != nil {
		return nil, err
	}
	watcher, ok := r.resources.Get(id).(state.RelationUnitsWatcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvRelationUnitsWatcher{
		watcher:   watcher,
		id:        id,
		resources: r.resources,
	}, nil
}

// AllWatcher returns an object that provides API access to methods on
// a state/multiwatcher.Watcher, which watches any changes to the
// state. Each client has its own current set of watchers, stored in
// r.resources.
func (r *srvRoot) AllWatcher(id string) (*srvClientAllWatcher, error) {
	if err := r.requireClient(); err != nil {
		return nil, err
	}
	watcher, ok := r.resources.Get(id).(*multiwatcher.Watcher)
	if !ok {
		return nil, common.ErrUnknownWatcher
	}
	return &srvClientAllWatcher{
		watcher:   watcher,
		id:        id,
		resources: r.resources,
	}, nil
}

// Pinger returns an object that can be pinged
// by calling its Ping method. If this method
// is not called frequently enough, the connection
// will be dropped.
func (r *srvRoot) Pinger(id string) (pinger, error) {
	pingTimeout, ok := r.resources.Get("pingTimeout").(pinger)
	if !ok {
		return nullPinger{}, nil
	}
	return pingTimeout, nil
}

type nullPinger struct{}

func (nullPinger) Ping() {}

// AuthMachineAgent returns whether the current client is a machine agent.
func (r *srvRoot) AuthMachineAgent() bool {
	_, ok := r.entity.(*state.Machine)
	return ok
}

// AuthUnitAgent returns whether the current client is a unit agent.
func (r *srvRoot) AuthUnitAgent() bool {
	_, ok := r.entity.(*state.Unit)
	return ok
}

// AuthOwner returns whether the authenticated user's tag matches the
// given entity tag.
func (r *srvRoot) AuthOwner(tag string) bool {
	return r.entity.Tag() == tag
}

// AuthEnvironManager returns whether the authenticated user is a
// machine with running the ManageEnviron job.
func (r *srvRoot) AuthEnvironManager() bool {
	return isMachineWithJob(r.entity, state.JobManageEnviron)
}

// AuthClient returns whether the authenticated entity is a client
// user.
func (r *srvRoot) AuthClient() bool {
	return !isAgent(r.entity)
}

// GetAuthTag returns the tag of the authenticated entity.
func (r *srvRoot) GetAuthTag() string {
	return r.entity.Tag()
}

// GetAuthEntity returns the authenticated entity.
func (r *srvRoot) GetAuthEntity() state.Entity {
	return r.entity
}

// pinger describes a type that can be pinged.
type pinger interface {
	Ping()
}

// pingTimeout listens for pings and will call the
// passed action in case of a timeout. This way broken
// or inactive connections can be closed.
type pingTimeout struct {
	tomb    tomb.Tomb
	action  func()
	timeout time.Duration
	reset   chan struct{}
}

// newPingTimeout returns a new pingTimeout instance
// that invokes the given action asynchronously if there
// is more than the given timeout interval between calls
// to its Ping method.
func newPingTimeout(action func(), timeout time.Duration) *pingTimeout {
	pt := &pingTimeout{
		action:  action,
		timeout: timeout,
		reset:   make(chan struct{}),
	}
	go func() {
		defer pt.tomb.Done()
		pt.tomb.Kill(pt.loop())
	}()
	return pt
}

// Ping is used by the client heartbeat monitor and resets
// the killer.
func (pt *pingTimeout) Ping() {
	select {
	case <-pt.tomb.Dying():
	case pt.reset <- struct{}{}:
	}
}

// Stop terminates the ping timeout.
func (pt *pingTimeout) Stop() error {
	pt.tomb.Kill(nil)
	return pt.tomb.Wait()
}

// loop waits for a reset signal, otherwise it performs
// the initially passed action.
func (pt *pingTimeout) loop() error {
	timer := time.NewTimer(pt.timeout)
	defer timer.Stop()
	for {
		select {
		case <-pt.tomb.Dying():
			return nil
		case <-timer.C:
			go pt.action()
			return errors.New("ping timeout")
		case <-pt.reset:
			timer.Reset(pt.timeout)
		}
	}
}
