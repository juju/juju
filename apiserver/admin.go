// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
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
			return fail, errors.Trace(err)
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

	serverOnlyLogin := a.root.modelUUID == ""

	entity, lastConnection, err := doCheckCreds(a.root.state, req, !serverOnlyLogin, a.srv.authCtxt)
	if err != nil {
		if err, ok := errors.Cause(err).(*common.DischargeRequiredError); ok {
			loginResult := params.LoginResultV1{
				DischargeRequired:       err.Macaroon,
				DischargeRequiredReason: err.Error(),
			}
			logger.Infof("login failed with discharge-required error: %v", err)
			return loginResult, nil
		}
		if a.maintenanceInProgress() {
			// An upgrade, restore or similar operation is in
			// progress. It is possible for logins to fail until this
			// is complete due to incomplete or updating data. Mask
			// transitory and potentially confusing errors from failed
			// logins with a more helpful one.
			return fail, MaintenanceNoLoginError
		}
		// Here we have a special case.  The machine agents that manage
		// models in the controller model need to be able to
		// open API connections to other models.  In those cases, we
		// need to look in the controller database to check the creds
		// against the machine if and only if the entity tag is a machine tag,
		// and the machine exists in the controller model, and the
		// machine has the manage state job.  If all those parts are valid, we
		// can then check the credentials against the controller model
		// machine.
		if kind != names.MachineTagKind {
			return fail, errors.Trace(err)
		}
		entity, err = a.checkCredsOfControllerMachine(req)
		if err != nil {
			return fail, errors.Trace(err)
		}
		// If we are here, then the entity will refer to a controller
		// machine in the controller model, and we don't need a pinger
		// for it as we already have one running in the machine agent api
		// worker for the controller model.
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
			return fail, errors.Trace(err)
		}
	}

	var maybeUserInfo *params.AuthUserInfo
	var envUser *state.ModelUser
	// Send back user info if user
	if isUser && !serverOnlyLogin {
		maybeUserInfo = &params.AuthUserInfo{
			Identity:       entity.Tag().String(),
			LastConnection: lastConnection,
		}
		envUser, err = a.root.state.ModelUser(entity.Tag().(names.UserTag))
		if err != nil {
			return fail, errors.Annotatef(err, "missing ModelUser for logged in user %s", entity.Tag())
		}
		if envUser.ReadOnly() {
			logger.Debugf("model user %s is READ ONLY", entity.Tag())
		}
	}

	// Fetch the API server addresses from state.
	hostPorts, err := a.root.state.APIHostPorts()
	if err != nil {
		return fail, errors.Trace(err)
	}
	logger.Debugf("hostPorts: %v", hostPorts)

	environ, err := a.root.state.Model()
	if err != nil {
		return fail, errors.Trace(err)
	}

	loginResult := params.LoginResultV1{
		Servers:       params.FromNetworkHostsPorts(hostPorts),
		ModelTag:      environ.Tag().String(),
		ControllerTag: environ.ControllerTag().String(),
		Facades:       DescribeFacades(),
		UserInfo:      maybeUserInfo,
		ServerVersion: version.Current.String(),
	}

	// For sufficiently modern login versions, stop serving the
	// controller model at the root of the API.
	if serverOnlyLogin {
		authedApi = newRestrictedRoot(authedApi)
		// Remove the ModelTag from the response as there is no
		// model here.
		loginResult.ModelTag = ""
		// Strip out the facades that are not supported from the result.
		var facades []params.FacadeVersions
		for _, facade := range loginResult.Facades {
			if restrictedRootNames.Contains(facade.Name) {
				facades = append(facades, facade)
			}
		}
		loginResult.Facades = facades
	}

	if envUser != nil {
		authedApi = newClientAuthRoot(authedApi, envUser)
	}

	a.root.rpcConn.ServeFinder(authedApi, serverError)

	return loginResult, nil
}

// checkCredsOfControllerMachine checks the special case of a controller
// machine creating an API connection for a different model so it can
// run API workers for that model to do things like provisioning
// machines.
func (a *admin) checkCredsOfControllerMachine(req params.LoginRequest) (state.Entity, error) {
	entity, _, err := doCheckCreds(a.srv.state, req, false, a.srv.authCtxt)
	if err != nil {
		return nil, errors.Trace(err)
	}
	machine, ok := entity.(*state.Machine)
	if !ok {
		return nil, errors.Errorf("entity should be a machine, but is %T", entity)
	}
	for _, job := range machine.Jobs() {
		if job == state.JobManageModel {
			return entity, nil
		}
	}
	// The machine does exist in the controller model, but it
	// doesn't manage models, so reject it.
	return nil, errors.Trace(common.ErrBadCreds)
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

// checkCreds validates the entities credentials in the current model.
// If the entity is a user, and lookForModelUser is true, a model user must exist
// for the model.  In the case of a user logging in to the server, but
// not a model, there is no env user needed.  While we have the env
// user, if we do have it, update the last login time.
//
// Note that when logging in with lookForModelUser true, the returned
// entity will be modelUserEntity, not *state.User (external users
// don't have user entries) or *state.ModelUser (we
// don't want to lose the local user information associated with that).
func checkCreds(st *state.State, req params.LoginRequest, lookForModelUser bool, authenticator authentication.EntityAuthenticator) (state.Entity, *time.Time, error) {
	var tag names.Tag
	if req.AuthTag != "" {
		var err error
		tag, err = names.ParseTag(req.AuthTag)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
	}
	var entityFinder authentication.EntityFinder = st
	if lookForModelUser {
		// When looking up model users, use a custom
		// entity finder that looks up both the local user (if the user
		// tag is in the local domain) and the model user.
		entityFinder = modelUserEntityFinder{st}
	}
	entity, err := authenticator.Authenticate(entityFinder, tag, req)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// For user logins, update the last login time.
	var lastLogin *time.Time
	if entity, ok := entity.(loginEntity); ok {
		userLastLogin, err := entity.LastLogin()
		if err != nil && !state.IsNeverLoggedInError(err) {
			return nil, nil, errors.Trace(err)
		}
		entity.UpdateLastLogin()
		lastLogin = &userLastLogin
	}
	return entity, lastLogin, nil
}

// loginEntity defines the interface needed to log in as a user.
// Notable implementations are *state.User and *modelUserEntity.
type loginEntity interface {
	state.Entity
	state.Authenticator
	LastLogin() (time.Time, error)
	UpdateLastLogin() error
}

// modelUserEntityFinder implements EntityFinder by returning a
// loginEntity value for users, ensuring that the user exists in the
// state's current model as well as retrieving more global
// authentication details such as the password.
type modelUserEntityFinder struct {
	st *state.State
}

// FindEntity implements authentication.EntityFinder.FindEntity.
func (f modelUserEntityFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	utag, ok := tag.(names.UserTag)
	if !ok {
		return f.st.FindEntity(tag)
	}
	modelUser, err := f.st.ModelUser(utag)
	if err != nil {
		return nil, err
	}
	u := &modelUserEntity{
		modelUser: modelUser,
	}
	if utag.IsLocal() {
		user, err := f.st.User(utag)
		if err != nil {
			return nil, err
		}
		u.user = user
	}
	return u, nil
}

var _ loginEntity = &modelUserEntity{}

// modelUserEntity encapsulates an model user
// and, if the user is local, the local state user
// as well. This enables us to implement FindEntity
// in such a way that the authentication mechanisms
// can work without knowing these details.
type modelUserEntity struct {
	modelUser *state.ModelUser
	user      *state.User
}

// Refresh implements state.Authenticator.Refresh.
func (u *modelUserEntity) Refresh() error {
	if u.user == nil {
		return nil
	}
	return u.user.Refresh()
}

// SetPassword implements state.Authenticator.SetPassword
// by setting the password on the local user.
func (u *modelUserEntity) SetPassword(pass string) error {
	if u.user == nil {
		return errors.New("cannot set password on external user")
	}
	return u.user.SetPassword(pass)
}

// PasswordValid implements state.Authenticator.PasswordValid.
func (u *modelUserEntity) PasswordValid(pass string) bool {
	if u.user == nil {
		return false
	}
	return u.user.PasswordValid(pass)
}

// Tag implements state.Entity.Tag.
func (u *modelUserEntity) Tag() names.Tag {
	return u.modelUser.UserTag()
}

// LastLogin implements loginEntity.LastLogin.
func (u *modelUserEntity) LastLogin() (time.Time, error) {
	// The last connection for the model takes precedence over
	// the local user last login time.
	t, err := u.modelUser.LastConnection()
	if state.IsNeverConnectedError(err) {
		if u.user != nil {
			// There's a global user, so use that login time instead.
			return u.user.LastLogin()
		}
		// Since we're implementing LastLogin, we need
		// to implement LastLogin error semantics too.
		err = state.NeverLoggedInError(err.Error())
	}
	return t, err
}

// UpdateLastLogin implements loginEntity.UpdateLastLogin.
func (u *modelUserEntity) UpdateLastLogin() error {
	err := u.modelUser.UpdateLastConnection()
	if u.user != nil {
		err1 := u.user.UpdateLastLogin()
		if err == nil {
			err = err1
		}
	}
	return err
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
	mongoUnavailable *uint32
}

// Stop implements Pinger.Stop() as Pinger.Kill(), needed at
// connection closing time to properly stop the wrapped pinger.
func (p *machinePinger) Stop() error {
	if err := p.Pinger.Stop(); err != nil {
		return err
	}
	if atomic.LoadUint32(p.mongoUnavailable) > 0 {
		// Kill marks the agent as not-present. If the
		// Mongo server is known to be unavailable, then
		// we do not perform this operation; the agent
		// will naturally become "not present" when its
		// presence expires.
		return nil
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

	root.getResources().Register(&machinePinger{pinger, root.mongoUnavailable})
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

// FindMethod conforms to the same API as initialRoot, but we'll always return (nil, err)
func (r *errRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	return nil, r.err
}
