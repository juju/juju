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
)

type adminApiFactory func(srv *Server, root *apiHandler, reqNotifier *requestNotifier) interface{}

// adminApiV0 implements the API that a client first sees when connecting to
// the API. We start serving a different API once the user has logged in.
type adminApiV0 struct {
	admin *adminV0
}

// admin is the only object that unlogged-in clients can access. It holds any
// methods that are needed to log in.
type admin struct {
	srv         *Server
	root        *apiHandler
	reqNotifier *requestNotifier

	mu       sync.Mutex
	loggedIn bool
}

type adminV0 struct {
	*admin
}

func newAdminApiV0(srv *Server, root *apiHandler, reqNotifier *requestNotifier) interface{} {
	return &adminApiV0{
		admin: &adminV0{
			&admin{
				srv:         srv,
				root:        root,
				reqNotifier: reqNotifier,
			},
		},
	}
}

// Admin returns an object that provides API access to methods that can be
// called even when not authenticated.
func (r *adminApiV0) Admin(id string) (*adminV0, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return r.admin, nil
}

var UpgradeInProgressError = errors.New("upgrade in progress")
var AboutToRestoreError = errors.New("restore preparation in progress")
var RestoreInProgressError = errors.New("restore in progress")
var MaintenanceNoLoginError = errors.New("login failed - maintenance in progress")
var errAlreadyLoggedIn = errors.New("already logged in")

// Login logs in with the provided credentials.  All subsequent requests on the
// connection will act as the authenticated user.
func (a *adminV0) Login(c params.Creds) (params.LoginResult, error) {
	var fail params.LoginResult

	resultV1, err := a.doLogin(params.LoginRequest{
		AuthTag:     c.AuthTag,
		Credentials: c.Password,
		Nonce:       c.Nonce,
	})
	if err != nil {
		return fail, err
	}

	resultV0 := params.LoginResult{
		Servers:    resultV1.Servers,
		EnvironTag: resultV1.EnvironTag,
		Facades:    resultV1.Facades,
	}
	if resultV1.UserInfo != nil {
		resultV0.LastConnection = resultV1.UserInfo.LastConnection
	}
	return resultV0, nil
}

func (a *admin) doLogin(req params.LoginRequest) (params.LoginResultV1, error) {
	var fail params.LoginResultV1

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loggedIn {
		// This can only happen if Login is called concurrently.
		return fail, errAlreadyLoggedIn
	}

	// authedApi is the API method finder we'll use after getting logged in.
	var authedApi rpc.MethodFinder = newApiRoot(a.srv, a.root.resources, a.root)

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

	var isUser bool
	if kind, err := names.TagKind(req.AuthTag); err != nil || kind != names.UserTagKind {
		// Users are not rate limited, all other entities are
		if !a.srv.limiter.Acquire() {
			logger.Debugf("rate limiting, try again later")
			return fail, common.ErrTryAgain
		}
		defer a.srv.limiter.Release()
	} else {
		isUser = true
	}

	entity, err := doCheckCreds(a.srv.state, req)
	if err != nil {
		if a.maintenanceInProgress() {
			// An upgrade, restore or similar operation is in
			// progress. It is possible for logins to fail until this
			// is complete due to incomplete or updating data. Mask
			// transitory and potentially confusing errors from failed
			// logins with a more helpful one.
			return fail, MaintenanceNoLoginError
		} else {
			return fail, err
		}
		return fail, err
	}
	a.root.entity = entity

	if a.reqNotifier != nil {
		a.reqNotifier.login(entity.Tag().String())
	}

	// We have authenticated the user; enable the appropriate API
	// to serve to them.
	a.loggedIn = true

	if err := startPingerIfAgent(a.root, entity); err != nil {
		return fail, err
	}

	var maybeUserInfo *params.UserInfo
	// Send back user info if user
	if isUser {
		lastConnection := getAndUpdateLastLoginForEntity(entity)
		maybeUserInfo = &params.UserInfo{
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

	a.root.rpcConn.ServeFinder(authedApi, serverError)

	return params.LoginResultV1{
		Servers:    hostPorts,
		EnvironTag: environ.Tag().String(),
		Facades:    DescribeFacades(),
		UserInfo:   maybeUserInfo,
	}, nil
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

func checkCreds(st *state.State, req params.LoginRequest) (state.Entity, error) {
	tag, err := names.ParseTag(req.AuthTag)
	if err != nil {
		return nil, err
	}
	entity, err := st.FindEntity(tag)
	if errors.IsNotFound(err) {
		// We return the same error when an entity does not exist as for a bad
		// password, so that we don't allow unauthenticated users to find
		// information about existing entities.
		logger.Debugf("entity %q not found", tag)
		return nil, common.ErrBadCreds
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	authenticator, err := authentication.FindEntityAuthenticator(entity)
	if err != nil {
		return nil, err
	}

	if err = authenticator.Authenticate(entity, req.Credentials, req.Nonce); err != nil {
		logger.Debugf("bad credentials")
		return nil, err
	}

	// For user logins, ensure the user is allowed to access the environment.
	if user, ok := entity.Tag().(names.UserTag); ok {
		_, err := st.EnvironmentUser(user)
		if err != nil {
			return nil, errors.Wrap(err, common.ErrBadCreds)
		}
	}

	return entity, nil
}

func getAndUpdateLastLoginForEntity(entity state.Entity) *time.Time {
	if user, ok := entity.(*state.User); ok {
		result := user.LastLogin()
		user.UpdateLastLogin()
		return result
	}
	return nil
}

func checkForValidMachineAgent(entity state.Entity, req params.LoginRequest) error {
	// If this is a machine agent connecting, we need to check the
	// nonce matches, otherwise the wrong agent might be trying to
	// connect.
	if machine, ok := entity.(*state.Machine); ok {
		if !machine.CheckProvisioned(req.Nonce) {
			return state.NotProvisionedError(machine.Id())
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
	err = root.getResources().RegisterNamed("pingTimeout", pingTimeout)
	if err != nil {
		return err
	}
	return nil
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
