// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/presence"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/juju/state"
	statepresence "github.com/juju/juju/state/presence"
	jujuversion "github.com/juju/juju/version"
)

type adminAPIFactory func(*Server, *apiHandler, observer.Observer) interface{}

// admin is the only object that unlogged-in clients can access. It holds any
// methods that are needed to log in.
type admin struct {
	srv         *Server
	root        *apiHandler
	apiObserver observer.Observer

	mu       sync.Mutex
	loggedIn bool
}

var AboutToRestoreError = errors.New("restore preparation in progress")
var RestoreInProgressError = errors.New("restore in progress")
var MaintenanceNoLoginError = errors.New("login failed - maintenance in progress")
var errAlreadyLoggedIn = errors.New("already logged in")

// login is the internal version of the Login API call.
func (a *admin) login(req params.LoginRequest, loginVersion int) (params.LoginResult, error) {
	var fail params.LoginResult

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loggedIn {
		// This can only happen if Login is called concurrently.
		return fail, errAlreadyLoggedIn
	}

	// apiRoot is the API root exposed to the client after authentication.
	var apiRoot rpc.Root = newAPIRoot(a.root.state, a.root.resources, a.root)

	// Use the login validation function, if one was specified.
	if a.srv.validator != nil {
		err := a.srv.validator(req)
		switch err {
		case params.UpgradeInProgressError:
			apiRoot = restrictRoot(apiRoot, upgradeMethodsOnly)
		case AboutToRestoreError:
			apiRoot = restrictRoot(apiRoot, aboutToRestoreMethodsOnly)
		case RestoreInProgressError:
			apiRoot = restrictAll(apiRoot, restoreInProgressError)
		case nil:
			// in this case no need to wrap authed api so we do nothing
		default:
			return fail, errors.Trace(err)
		}
	}

	isUser := true
	kind := names.UserTagKind
	if req.AuthTag != "" {
		var err error
		kind, err = names.TagKind(req.AuthTag)
		if err != nil || kind != names.UserTagKind {
			isUser = false
			// Users are not rate limited, all other entities are.
			if !a.srv.limiter.Acquire() {
				logger.Debugf("rate limiting for agent %s", req.AuthTag)
				return fail, common.ErrTryAgain
			}
			defer a.srv.limiter.Release()
		}
	}

	controllerOnlyLogin := a.root.modelUUID == ""
	controllerMachineLogin := false

	entity, lastConnection, err := doCheckCreds(a.root.state, req, isUser, a.srv.authCtxt)
	if err != nil {
		if err, ok := errors.Cause(err).(*common.DischargeRequiredError); ok {
			loginResult := params.LoginResult{
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
		if errors.Cause(err) != common.ErrBadCreds {
			return fail, err
		}
		entity, err = a.checkControllerMachineCreds(req)
		if err != nil {
			return fail, errors.Trace(err)
		}
		// If we are here, then the entity will refer to a controller
		// machine in the controller model, and we don't need a pinger
		// for it as we already have one running in the machine agent api
		// worker for the controller model.
		controllerMachineLogin = true
	}
	a.root.entity = entity
	a.apiObserver.Login(entity.Tag(), a.root.state.ModelTag(), controllerMachineLogin, req.UserData)

	// We have authenticated the user; enable the appropriate API
	// to serve to them.
	a.loggedIn = true

	if !controllerMachineLogin {
		if err := startPingerIfAgent(a.root, entity); err != nil {
			return fail, errors.Trace(err)
		}
	}

	var maybeUserInfo *params.AuthUserInfo
	var modelUser description.UserAccess
	var everyoneGroupUser description.UserAccess
	// Send back user info if user
	if isUser {
		maybeUserInfo = &params.AuthUserInfo{
			Identity:       entity.Tag().String(),
			LastConnection: lastConnection,
		}
		userTag := entity.Tag().(names.UserTag)

		// TODO(perrito666) remove the following section about everyone group
		// when groups are implemented, this accounts only for the lack of a local
		// ControllerUser when logging in from an external user that has not been granted
		// permissions on the controller but there are permissions for the special
		// everyone group.
		if !userTag.IsLocal() {
			everyoneTag := names.NewUserTag(common.EveryoneTagName)
			everyoneGroupUser, err = state.ControllerAccess(a.root.state, everyoneTag)
			if err != nil && !errors.IsNotFound(err) {
				return fail, errors.Annotatef(err, "obtaining ControllerUser for everyone group")
			}
		}

		modelUser, err = a.root.state.UserAccess(userTag, a.root.state.ModelTag())
		if err != nil && !errors.IsNotFound(err) {
			return fail, errors.Annotatef(err, "obtaining ModelUser for logged in user %s", entity.Tag())
		}

		controllerUser, err := state.ControllerAccess(a.root.state, entity.Tag())
		if err != nil && !errors.IsNotFound(err) {
			return fail, errors.Annotatef(err, "obtaining ControllerUser for logged in user %s", entity.Tag())
		}

		if description.IsEmptyUserAccess(modelUser) &&
			description.IsEmptyUserAccess(controllerUser) &&
			description.IsEmptyUserAccess(everyoneGroupUser) {
			return fail, errors.NotFoundf("model or controller access for logged in user %q", userTag.Canonical())
		}
		maybeUserInfo.ControllerAccess = string(controllerUser.Access)
		maybeUserInfo.ModelAccess = string(modelUser.Access)
		logger.Tracef("controller user %s has %v", entity.Tag(), controllerUser.Access)
		logger.Tracef("model user %s has %s", entity.Tag(), modelUser.Access)
	}

	// Fetch the API server addresses from state.
	hostPorts, err := a.root.state.APIHostPorts()
	if err != nil {
		return fail, errors.Trace(err)
	}
	logger.Debugf("hostPorts: %v", hostPorts)

	model, err := a.root.state.Model()
	if err != nil {
		return fail, errors.Trace(err)
	}

	if isUser && model.MigrationMode() == state.MigrationModeImporting {
		apiRoot = restrictAll(apiRoot, errors.New("migration in progress, model is importing"))
	}

	loginResult := params.LoginResult{
		Servers:       params.FromNetworkHostsPorts(hostPorts),
		ControllerTag: model.ControllerTag().String(),
		UserInfo:      maybeUserInfo,
		ServerVersion: jujuversion.Current.String(),
	}

	if controllerOnlyLogin {
		loginResult.Facades = filterFacades(isControllerFacade)
		apiRoot = restrictRoot(apiRoot, controllerFacadesOnly)
	} else {
		loginResult.ModelTag = model.Tag().String()
		loginResult.Facades = filterFacades(isModelFacade)
		apiRoot = restrictRoot(apiRoot, modelFacadesOnly)
	}

	a.root.rpcConn.ServeRoot(apiRoot, serverError)

	return loginResult, nil
}

func filterFacades(allowFacade func(name string) bool) []params.FacadeVersions {
	allFacades := DescribeFacades()
	out := make([]params.FacadeVersions, 0, len(allFacades))
	for _, facade := range allFacades {
		if allowFacade(facade.Name) {
			out = append(out, facade)
		}
	}
	return out
}

func (a *admin) checkControllerMachineCreds(req params.LoginRequest) (state.Entity, error) {
	return checkControllerMachineCreds(a.srv.state, req, a.srv.authCtxt)
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
// for the model.  In the case of a user logging in to the controller, but
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

// checkControllerMachineCreds checks the special case of a controller
// machine creating an API connection for a different model so it can
// run workers that act on behalf of a hosted model.
func checkControllerMachineCreds(
	controllerSt *state.State,
	req params.LoginRequest,
	authenticator authentication.EntityAuthenticator,
) (state.Entity, error) {
	entity, _, err := doCheckCreds(controllerSt, req, false, authenticator)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if machine, ok := entity.(*state.Machine); !ok {
		return nil, errors.Errorf("entity should be a machine, but is %T", entity)
	} else if !machine.IsManager() {
		// The machine exists in the controller model, but it doesn't
		// manage models, so reject it.
		return nil, errors.Trace(common.ErrPerm)
	}
	return entity, nil
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

	modelUser, controllerUser, err := common.UserAccess(f.st, utag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	u := &modelUserEntity{
		st:             f.st,
		modelUser:      modelUser,
		controllerUser: controllerUser,
	}
	if utag.IsLocal() {
		user, err := f.st.User(utag)
		if err != nil {
			return nil, errors.Trace(err)
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
	st *state.State

	controllerUser description.UserAccess
	modelUser      description.UserAccess
	user           *state.User
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
	if u.user != nil {
		return u.user.UserTag()
	}
	if !description.IsEmptyUserAccess(u.modelUser) {
		return u.modelUser.UserTag
	}
	return u.controllerUser.UserTag

}

// LastLogin implements loginEntity.LastLogin.
func (u *modelUserEntity) LastLogin() (time.Time, error) {
	// The last connection for the model takes precedence over
	// the local user last login time.
	var err error
	var t time.Time
	if !description.IsEmptyUserAccess(u.modelUser) {
		t, err = u.st.LastModelConnection(u.modelUser.UserTag)
	} else {
		err = state.NeverConnectedError("controller user")
	}
	if state.IsNeverConnectedError(err) || description.IsEmptyUserAccess(u.modelUser) {
		if u.user != nil {
			// There's a global user, so use that login time instead.
			return u.user.LastLogin()
		}
		// Since we're implementing LastLogin, we need
		// to implement LastLogin error semantics too.
		err = state.NeverLoggedInError(err.Error())
	}
	return t, errors.Trace(err)
}

// UpdateLastLogin implements loginEntity.UpdateLastLogin.
func (u *modelUserEntity) UpdateLastLogin() error {
	var err error

	if !description.IsEmptyUserAccess(u.modelUser) {
		if u.modelUser.Object.Kind() != names.ModelTagKind {
			return errors.NotValidf("%s as model user", u.modelUser.Object.Kind())
		}

		err = u.st.UpdateLastModelConnection(u.modelUser.UserTag)
	}

	if u.user != nil {
		err1 := u.user.UpdateLastLogin()
		if err == nil {
			return err1
		}
	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// presenceShim exists to represent a statepresence.Agent in a form
// convenient to the apiserver/presence package, which exists to work
// around the common.Resources infrastructure's lack of handling for
// failed resources.
type presenceShim struct {
	agent statepresence.Agent
}

// Start starts and returns a running presence.Pinger. The caller is
// responsible for stopping it when no longer required, and for handling
// any errors returned from Wait.
func (shim presenceShim) Start() (presence.Pinger, error) {
	pinger, err := shim.agent.SetAgentPresence()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return pinger, nil
}

func startPingerIfAgent(root *apiHandler, entity state.Entity) error {
	// worker runs presence.Pingers -- absence of which will cause
	// embarrassing "agent is lost" messages to show up in status --
	// until it's stopped. It's stored in resources purely for the
	// side effects: we don't record its id, and nobody else
	// retrieves it -- we just expect it to be stopped when the
	// connection is shut down.
	agent, ok := entity.(statepresence.Agent)
	if !ok {
		return nil
	}
	worker, err := presence.New(presence.Config{
		Identity:   entity.Tag(),
		Start:      presenceShim{agent}.Start,
		Clock:      clock.WallClock,
		RetryDelay: 3 * time.Second,
	})
	if err != nil {
		return err
	}
	root.getResources().Register(worker)

	// pingTimeout, by contrast, *is* used by the Pinger facade to
	// stave off the call to action() that will shut down the agent
	// connection if it gets lackadaisical about sending keepalive
	// Pings.
	//
	// Do not confuse those (apiserver) Pings with those made by
	// presence.Pinger (which *do* happen as a result of the former,
	// but only as a relatively distant consequence).
	//
	// We should have picked better names...
	action := func() {
		logger.Debugf("closing connection due to ping timout")
		if err := root.getRpcConn().Close(); err != nil {
			logger.Errorf("error closing the RPC connection: %v", err)
		}
	}
	pingTimeout := newPingTimeout(action, clock.WallClock, maxClientPingInterval)
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

func (r *errRoot) Kill() {
}
