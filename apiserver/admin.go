// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/version"
)

type adminApiFactory func(srv *Server, root *apiHandler, reqNotifier *requestNotifier) interface{}

// admin is the only object that unlogged-in clients can access. It holds any
// methods that are needed to log in.
type admin struct {
	srv         *Server
	root        *apiHandler
	reqNotifier *requestNotifier

	mu       sync.Mutex
	loggedIn bool
}

var UpgradeInProgressError = errors.New("upgrade in progress")
var AboutToRestoreError = errors.New("restore preparation in progress")
var RestoreInProgressError = errors.New("restore in progress")
var MaintenanceNoLoginError = errors.New("login failed - maintenance in progress")
var errAlreadyLoggedIn = errors.New("already logged in")

func (a *admin) doLogin(req params.LoginRequest, loginVersion int) (params.LoginResultV1, error) {
	var fail params.LoginResultV1

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loggedIn {
		// This can only happen if Login is called concurrently.
		return fail, errAlreadyLoggedIn
	}

	// authedApi is the API method finder we'll use after getting logged in.
	var authedApi rpc.MethodFinder = newApiRoot(a.root.state, a.root.resources, a.root)

	// Use the login validation function, if one was specified.
	if a.srv.validator != nil {
		err := a.srv.validator(req)
		switch err {
		case UpgradeInProgressError:
			authedApi = newUpgradingRoot(authedApi)
		case AboutToRestoreError:
			authedApi = newAboutToRestoreRoot(authedApi)
		case RestoreInProgressError:
			authedApi = newRestoreInProgressRoot(authedApi)
		case nil:
			// in this case no need to wrap authed api so we do nothing
		default:
			return fail, err
		}
	}

	var agentPingerNeeded = true
	var isUser bool
	kind, err := names.TagKind(req.AuthTag)
	if err != nil || kind != names.UserTagKind {
		// Users are not rate limited, all other entities are
		if !a.srv.limiter.Acquire() {
			logger.Debugf("rate limiting for agent %s", req.AuthTag)
			return fail, common.ErrTryAgain
		}
		defer a.srv.limiter.Release()
	} else {
		isUser = true
	}

	serverOnlyLogin := loginVersion > 1 && a.root.envUUID == ""

	entity, lastConnection, err := doCheckCreds(a.root.state, req, !serverOnlyLogin)
	if err != nil {
		if a.maintenanceInProgress() {
			// An upgrade, restore or similar operation is in
			// progress. It is possible for logins to fail until this
			// is complete due to incomplete or updating data. Mask
			// transitory and potentially confusing errors from failed
			// logins with a more helpful one.
			return fail, MaintenanceNoLoginError
		}
		// Here we have a special case.  The machine agents that manage
		// environments in the state server environment need to be able to
		// open API connections to other environments.  In those cases, we
		// need to look in the state server database to check the creds
		// against the machine if and only if the entity tag is a machine tag,
		// and the machine exists in the state server environment, and the
		// machine has the manage state job.  If all those parts are valid, we
		// can then check the credentials against the state server environment
		// machine.
		if kind != names.MachineTagKind {
			return fail, err
		}
		entity, err = a.checkCredsOfStateServerMachine(req)
		if err != nil {
			return fail, err
		}
		// If we are here, then the entity will refer to a state server
		// machine in the state server environment, and we don't need a pinger
		// for it as we already have one running in the machine agent api
		// worker for the state server environment.
		agentPingerNeeded = false
	}
	a.root.entity = entity

	if a.reqNotifier != nil {
		a.reqNotifier.login(entity.Tag().String())
	}

	// We have authenticated the user; enable the appropriate API
	// to serve to them.
	a.loggedIn = true

	if agentPingerNeeded {
		if err := startPingerIfAgent(a.root, entity); err != nil {
			return fail, err
		}
	}

	var maybeUserInfo *params.AuthUserInfo
	// Send back user info if user
	if isUser {
		maybeUserInfo = &params.AuthUserInfo{
			Identity:       entity.Tag().String(),
			LastConnection: lastConnection,
		}
	}

	// Fetch the API server addresses from state.
	hostPorts, err := a.root.state.APIHostPorts()
	if err != nil {
		return fail, err
	}
	logger.Debugf("hostPorts: %v", hostPorts)

	environ, err := a.root.state.Environment()
	if err != nil {
		return fail, err
	}

	loginResult := params.LoginResultV1{
		Servers:       params.FromNetworkHostsPorts(hostPorts),
		EnvironTag:    environ.Tag().String(),
		ServerTag:     environ.ServerTag().String(),
		Facades:       DescribeFacades(),
		UserInfo:      maybeUserInfo,
		ServerVersion: version.Current.Number.String(),
	}

	// For sufficiently modern login versions, stop serving the
	// state server environment at the root of the API.
	if serverOnlyLogin {
		authedApi = newRestrictedRoot(authedApi)
		// Remove the EnvironTag from the response as there is no
		// environment here.
		loginResult.EnvironTag = ""
		// Strip out the facades that are not supported from the result.
		var facades []params.FacadeVersions
		for _, facade := range loginResult.Facades {
			if restrictedRootNames.Contains(facade.Name) {
				facades = append(facades, facade)
			}
		}
		loginResult.Facades = facades
	}

	a.root.rpcConn.ServeFinder(authedApi, serverError)

	return loginResult, nil
}

// checkCredsOfStateServerMachine checks the special case of a state server
// machine creating an API connection for a different environment so it can
// run API workers for that environment to do things like provisioning
// machines.
func (a *admin) checkCredsOfStateServerMachine(req params.LoginRequest) (state.Entity, error) {
	// Check the credentials against the state server environment.
	entity, _, err := doCheckCreds(a.srv.state, req, false)
	if err != nil {
		return nil, err
	}
	machine, ok := entity.(*state.Machine)
	if !ok {
		return nil, errors.Errorf("entity should be a machine, but is %T", entity)
	}
	for _, job := range machine.Jobs() {
		if job == state.JobManageEnviron {
			return entity, nil
		}
	}
	// The machine does exist in the state server environment, but it
	// doesn't manage environments, so reject it.
	return nil, common.ErrBadCreds
}

func (a *admin) maintenanceInProgress() bool {
	if a.srv.validator == nil {
		return false
	}
	// jujud's login validator will return an error for any user tag
	// if jujud is upgrading or restoring. The tag of the entity
	// trying to log in can't be used because jujud's login validator
	// will always return nil for the local machine agent and here we
	// need to know if maintenance is in progress irrespective of the
	// the authenticating entity.
	//
	// TODO(mjs): 2014-09-29 bug 1375110
	// This needs improving but I don't have the cycles right now.
	req := params.LoginRequest{
		AuthTag: names.NewUserTag("arbitrary").String(),
	}
	return a.srv.validator(req) != nil
}

var doCheckCreds = checkCreds

// checkCreds validates the entities credentials in the current environment.
// If the entity is a user, and lookForEnvUser is true, an env user must exist
// for the environment.  In the case of a user logging in to the server, but
// not an environment, there is no env user needed.  While we have the env
// user, if we do have it, update the last login time.
func checkCreds(st *state.State, req params.LoginRequest, lookForEnvUser bool) (state.Entity, *time.Time, error) {
	tag, err := names.ParseTag(req.AuthTag)
	if err != nil {
		return nil, nil, err
	}
	entity, err := st.FindEntity(tag)
	if errors.IsNotFound(err) {
		// We return the same error when an entity does not exist as for a bad
		// password, so that we don't allow unauthenticated users to find
		// information about existing entities.
		logger.Debugf("entity %q not found", tag)
		return nil, nil, common.ErrBadCreds
	}
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	authenticator, err := authentication.FindEntityAuthenticator(entity)
	if err != nil {
		return nil, nil, err
	}

	if err = authenticator.Authenticate(entity, req.Credentials, req.Nonce); err != nil {
		logger.Debugf("bad credentials")
		return nil, nil, err
	}

	// For user logins, update the last login time.
	// NOTE: this code path is only for local users. When we support remote
	// user logins with bearer tokens, we will need to make sure that we also
	// update the last connection times for the environment users there.
	var lastLogin *time.Time
	if user, ok := entity.(*state.User); ok {
		userLastLogin, err := user.LastLogin()
		if err != nil && !state.IsNeverLoggedInError(err) {
			return nil, nil, errors.Trace(err)
		}
		if lookForEnvUser {
			envUser, err := st.EnvironmentUser(user.UserTag())
			if err != nil {
				return nil, nil, errors.Wrap(err, common.ErrBadCreds)
			}
			// The last connection for the environment takes precedence over
			// the local user last login time.
			userLastLogin, err = envUser.LastConnection()
			if err != nil && !state.IsNeverConnectedError(err) {
				return nil, nil, errors.Trace(err)
			}
			envUser.UpdateLastConnection()
		}
		// Only update the user's last login time if it is a successful
		// login, meaning that if we are logging into an environment, make
		// sure that there is an environment user in that environment for
		// this user.
		user.UpdateLastLogin()
		lastLogin = &userLastLogin
	}

	return entity, lastLogin, nil
}

func checkForValidMachineAgent(entity state.Entity, req params.LoginRequest) error {
	// If this is a machine agent connecting, we need to check the
	// nonce matches, otherwise the wrong agent might be trying to
	// connect.
	if machine, ok := entity.(*state.Machine); ok {
		if !machine.CheckProvisioned(req.Nonce) {
			return errors.NotProvisionedf("machine %v", machine.Id())
		}
	}
	return nil
}

// machinePinger wraps a presence.Pinger.
type machinePinger struct {
	*presence.Pinger
}

// Stop implements Pinger.Stop() as Pinger.Kill(), needed at
// connection closing time to properly stop the wrapped pinger.
func (p *machinePinger) Stop() error {
	if err := p.Pinger.Stop(); err != nil {
		return err
	}
	return p.Pinger.Kill()
}

func startPingerIfAgent(root *apiHandler, entity state.Entity) error {
	// A machine or unit agent has connected, so start a pinger to
	// announce it's now alive, and set up the API pinger
	// so that the connection will be terminated if a sufficient
	// interval passes between pings.
	agentPresencer, ok := entity.(presence.Presencer)
	if !ok {
		return nil
	}

	pinger, err := agentPresencer.SetAgentPresence()
	if err != nil {
		return err
	}

	root.getResources().Register(&machinePinger{pinger})
	action := func() {
		if err := root.getRpcConn().Close(); err != nil {
			logger.Errorf("error closing the RPC connection: %v", err)
		}
	}
	pingTimeout := newPingTimeout(action, maxClientPingInterval)
	return root.getResources().RegisterNamed("pingTimeout", pingTimeout)
}

// errRoot implements the API that a client first sees
// when connecting to the API. It exposes the same API as initialRoot, except
// it returns the requested error when the client makes any request.
type errRoot struct {
	err error
}

// Admin conforms to the same API as initialRoot, but we'll always return (nil, err)
func (r *errRoot) Admin(id string) (*adminV0, error) {
	return nil, r.err
}
