// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/rpcreflect"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
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

func newAdminAPIV3(srv *Server, root *apiHandler, apiObserver observer.Observer) interface{} {
	return &admin{
		srv:         srv,
		root:        root,
		apiObserver: apiObserver,
	}
}

// Admin returns an object that provides API access to methods that can be
// called even when not authenticated.
func (a *admin) Admin(id string) (*admin, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return a, nil
}

// Login logs in with the provided credentials.  All subsequent requests on the
// connection will act as the authenticated user.
func (a *admin) Login(req params.LoginRequest) (params.LoginResult, error) {
	return a.login(context.Background(), req, 3)
}

// RedirectInfo returns redirected host information for the model.
// In Juju it always returns an error because the Juju controller
// does not multiplex controllers.
func (a *admin) RedirectInfo() (params.RedirectInfoResult, error) {
	return params.RedirectInfoResult{}, fmt.Errorf("not redirected")
}

var MaintenanceNoLoginError = errors.New("login failed - maintenance in progress")
var errAlreadyLoggedIn = errors.New("already logged in")

// login is the internal version of the Login API call.
func (a *admin) login(ctx context.Context, req params.LoginRequest, loginVersion int) (params.LoginResult, error) {
	var fail params.LoginResult

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.loggedIn {
		// This can only happen if Login is called concurrently.
		return fail, errAlreadyLoggedIn
	}

	authResult, err := a.authenticate(ctx, req)
	if err, ok := errors.Cause(err).(*common.DischargeRequiredError); ok {
		loginResult := params.LoginResult{
			DischargeRequired:       err.LegacyMacaroon,
			BakeryDischargeRequired: err.Macaroon,
			DischargeRequiredReason: err.Error(),
		}
		logger.Infof("login failed with discharge-required error: %v", err)
		return loginResult, nil
	}
	if err != nil {
		return fail, errors.Trace(err)
	}

	// Fetch the API server addresses from state.
	// If the login comes from a client, return all available addresses.
	// Otherwise return the addresses suitable for agent use.
	getHostPorts := a.root.state.APIHostPortsForAgents
	if k, _ := names.TagKind(req.AuthTag); k == names.UserTagKind {
		getHostPorts = a.root.state.APIHostPortsForClients
	}
	hostPorts, err := getHostPorts()
	if err != nil {
		return fail, errors.Trace(err)
	}
	pServers := make([]network.HostPorts, len(hostPorts))
	for i, hps := range hostPorts {
		pServers[i] = hps.HostPorts()
	}

	// apiRoot is the API root exposed to the client after login.
	var apiRoot rpc.Root
	apiRoot, err = newAPIRoot(
		a.srv.clock,
		a.root.state,
		a.root.shared,
		a.srv.facades,
		a.root.resources,
		a.root,
	)
	if err != nil {
		return fail, errors.Trace(err)
	}
	apiRoot, err = restrictAPIRoot(
		a.srv,
		apiRoot,
		a.root.model,
		*authResult,
	)
	if err != nil {
		return fail, errors.Trace(err)
	}

	var facadeFilters []facadeFilterFunc
	var modelTag string
	if authResult.anonymousLogin {
		facadeFilters = append(facadeFilters, IsAnonymousFacade)
	}
	if authResult.controllerOnlyLogin {
		facadeFilters = append(facadeFilters, IsControllerFacade)
	} else {
		facadeFilters = append(facadeFilters, IsModelFacade)
		modelTag = a.root.model.Tag().String()
	}

	auditConfig := a.srv.GetAuditConfig()
	auditRecorder, err := a.getAuditRecorder(req, authResult, auditConfig)
	if err != nil {
		return fail, errors.Trace(err)
	}

	recorderFactory := observer.NewRecorderFactory(
		a.apiObserver, auditRecorder, auditConfig.CaptureAPIArgs,
	)
	a.root.rpcConn.ServeRoot(apiRoot, recorderFactory, serverError)
	return params.LoginResult{
		Servers:       params.FromHostsPorts(pServers),
		ControllerTag: a.root.model.ControllerTag().String(),
		UserInfo:      authResult.userInfo,
		ServerVersion: jujuversion.Current.String(),
		PublicDNSName: a.srv.publicDNSName(),
		ModelTag:      modelTag,
		Facades:       filterFacades(a.srv.facades, facadeFilters...),
	}, nil
}

func (a *admin) getAuditRecorder(req params.LoginRequest, authResult *authResult, cfg auditlog.Config) (*auditlog.Recorder, error) {
	if !authResult.userLogin || !cfg.Enabled {
		return nil, nil
	}
	// Wrap the audit logger in a filter that prevents us from logging
	// lots of readonly conversations (like "juju status" requests).
	filter := observer.MakeInterestingRequestFilter(cfg.ExcludeMethods)
	result, err := auditlog.NewRecorder(
		observer.NewAuditLogFilter(cfg.Target, filter),
		a.srv.clock,
		auditlog.ConversationArgs{
			Who:          a.root.entity.Tag().Id(),
			What:         req.CLIArgs,
			ModelName:    a.root.model.Name(),
			ModelUUID:    a.root.model.UUID(),
			ConnectionID: a.root.connectionID,
		},
	)
	if err != nil {
		logger.Errorf("couldn't add login to audit log: %+v", err)
		return nil, errors.Trace(err)
	}
	return result, nil
}

type authResult struct {
	tag                    names.Tag // nil if external user login
	anonymousLogin         bool
	userLogin              bool // false if anonymous user
	controllerOnlyLogin    bool
	controllerMachineLogin bool
	userInfo               *params.AuthUserInfo
}

func (a *admin) authenticate(ctx context.Context, req params.LoginRequest) (*authResult, error) {
	result := &authResult{
		controllerOnlyLogin: a.root.modelUUID == "",
		userLogin:           true,
	}

	if req.AuthTag != "" {
		tag, err := names.ParseTag(req.AuthTag)
		if err == nil {
			result.tag = tag
		}
		if err != nil || tag.Kind() != names.UserTagKind {
			// Either the tag is invalid, or
			// it's not a user; rate limit it.
			a.srv.metricsCollector.LoginAttempts.Inc()
			defer a.srv.metricsCollector.LoginAttempts.Dec()

			// Users are not rate limited, all other entities are.
			if err := a.srv.getAgentToken(); err != nil {
				logger.Tracef("rate limiting for agent %s", req.AuthTag)
				return nil, errors.Trace(err)
			}
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	// If the login attempt is for a migrated model,
	// a.root.model will be nil as the model document does not exist on this
	// controller and a.root.modelUUID cannot be resolved.
	// In this case use the requested model UUID to check if we need to return
	// a redirect error.
	modelUUID := a.root.modelUUID
	if a.root.model != nil {
		modelUUID = a.root.model.UUID()
	}
	if err := a.maybeEmitRedirectError(modelUUID, result.tag); err != nil {
		return nil, errors.Trace(err)
	}

	switch result.tag.(type) {
	case nil:
		// Macaroon logins are always for users.
	case names.UserTag:
		if result.tag.Id() == api.AnonymousUsername && len(req.Macaroons) == 0 {
			result.anonymousLogin = true
			result.userLogin = false
		}
	default:
		result.userLogin = false
	}

	// Anonymous logins come from other controllers (in cross-model relations).
	// We don't need to start pingers because we don't maintain presence
	// information for them.
	startPinger := !result.anonymousLogin

	// Only attempt to login with credentials if we are not doing an anonymous login.
	if !result.anonymousLogin {
		authInfo, err := a.srv.authenticator.AuthenticateLoginRequest(ctx, a.root.serverHost, modelUUID, req)
		if err != nil {
			return nil, a.handleAuthError(err)
		}

		result.controllerMachineLogin = authInfo.Controller
		// controllerConn is used to indicate a connection from
		// the controller to a non-controller model.
		controllerConn := false
		if authInfo.Controller && !a.root.state.IsController() {
			// We only need to run a pinger for controller machine
			// agents when logging into the controller model.
			startPinger = false
			controllerConn = true
		}

		// TODO(wallyworld) - we can't yet observe anonymous logins as entity must be non-nil
		a.root.entity = authInfo.Entity
		a.apiObserver.Login(authInfo.Entity.Tag(), a.root.model.ModelTag(), controllerConn, req.UserData)
	} else if a.root.model == nil {
		// Anonymous login to unknown model.
		// Hide the fact that the model does not exist.
		return nil, errors.Unauthorizedf("invalid entity name or password")
	}
	a.loggedIn = true

	if startPinger {
		if err := setupPingTimeoutDisconnect(a.srv.pingClock, a.root, a.root.entity); err != nil {
			return nil, errors.Trace(err)
		}
	}

	var lastConnection *time.Time
	if err := a.fillLoginDetails(result, lastConnection); err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

func (a *admin) maybeEmitRedirectError(modelUUID string, authTag names.Tag) error {
	userTag, ok := authTag.(names.UserTag)
	if !ok {
		return nil
	}

	st, err := a.root.shared.statePool.Get(modelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = st.Release() }()

	// If the model exists on this controller then no redirect is possible.
	if _, err := st.Model(); err == nil || !errors.IsNotFound(err) {
		return nil
	}

	// Check if the model was not found due to
	// being migrated to another controller.
	mig, err := st.CompletedMigration()
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}

	// If a user is trying to access a migrated model to which they are not
	// granted access, do not return a redirect error.
	// We need to return redirects if possible for anonymous logins in order
	// to ensure post-migration operation of CMRs.
	if mig == nil || (userTag.Id() != api.AnonymousUsername && mig.ModelUserAccess(userTag) == permission.NoAccess) {
		return nil
	}

	target, err := mig.TargetInfo()
	if err != nil {
		return errors.Trace(err)
	}

	hps, err := network.ParseProviderHostPorts(target.Addrs...)
	if err != nil {
		return errors.Trace(err)
	}

	return &common.RedirectError{
		Servers:         []network.ProviderHostPorts{hps},
		CACert:          target.CACert,
		ControllerTag:   target.ControllerTag,
		ControllerAlias: target.ControllerAlias,
	}
}

func (a *admin) handleAuthError(err error) error {
	if err, ok := errors.Cause(err).(*common.DischargeRequiredError); ok {
		return err
	}
	if a.maintenanceInProgress() {
		// An upgrade, restore or similar operation is in
		// progress. It is possible for logins to fail until this
		// is complete due to incomplete or updating data. Mask
		// transitory and potentially confusing errors from failed
		// logins with a more helpful one.
		return errors.Wrap(err, MaintenanceNoLoginError)
	}
	return err
}

func (a *admin) fillLoginDetails(result *authResult, lastConnection *time.Time) error {
	// Send back user info if user
	if result.userLogin {
		userTag := a.root.entity.Tag().(names.UserTag)
		var err error
		result.userInfo, err = a.checkUserPermissions(userTag, result.controllerOnlyLogin)
		if err != nil {
			return errors.Trace(err)
		}
		result.userInfo.LastConnection = lastConnection
	}
	if result.controllerOnlyLogin {
		if result.anonymousLogin {
			logger.Debugf(" anonymous controller login")
		} else {
			logger.Debugf("controller login: %s", a.root.entity.Tag())
		}
	} else {
		if result.anonymousLogin {
			logger.Debugf("anonymous model login")
		} else {
			logger.Debugf("model login: %s for %s", a.root.entity.Tag(), a.root.model.ModelTag().Id())
		}
	}
	return nil
}

func (a *admin) checkUserPermissions(userTag names.UserTag, controllerOnlyLogin bool) (*params.AuthUserInfo, error) {

	modelAccess := permission.NoAccess

	// TODO(perrito666) remove the following section about everyone group
	// when groups are implemented, this accounts only for the lack of a local
	// ControllerUser when logging in from an external user that has not been granted
	// permissions on the controller but there are permissions for the special
	// everyone group.
	everyoneGroupAccess := permission.NoAccess
	if !userTag.IsLocal() {
		everyoneTag := names.NewUserTag(common.EveryoneTagName)
		everyoneGroupUser, err := state.ControllerAccess(a.root.state, everyoneTag)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Annotatef(err, "obtaining ControllerUser for everyone group")
		}
		everyoneGroupAccess = everyoneGroupUser.Access
	}

	var controllerAccess permission.Access
	if controllerUser, err := state.ControllerAccess(a.root.state, userTag); err == nil {
		controllerAccess = controllerUser.Access
	} else if errors.IsNotFound(err) {
		controllerAccess = everyoneGroupAccess
	} else {
		return nil, errors.Annotatef(err, "obtaining ControllerUser for logged in user %s", userTag.Id())
	}
	if !controllerOnlyLogin {
		// Only grab modelUser permissions if this is not a controller only
		// login. In all situations, if the model user is not found, they have
		// no authorisation to access this model, unless the user is controller
		// admin.

		var err error
		modelAccess, err = a.root.state.UserPermission(userTag, a.root.model.ModelTag())
		if err != nil && controllerAccess != permission.SuperuserAccess {
			return nil, errors.Wrap(err, common.ErrPerm)
		}
		if err != nil && controllerAccess == permission.SuperuserAccess {
			modelAccess = permission.AdminAccess
		}
	}

	// It is possible that the everyoneGroup permissions are more capable than an
	// individuals. If they are, use them.
	if everyoneGroupAccess.GreaterControllerAccessThan(controllerAccess) {
		controllerAccess = everyoneGroupAccess
	}
	if controllerOnlyLogin || !a.srv.allowModelAccess {
		// We're either explicitly logging into the controller or
		// we must check that the user has access to the controller
		// even though they're logging into a model.
		if controllerAccess == permission.NoAccess {
			return nil, errors.Trace(common.ErrPerm)
		}
	}
	if controllerOnlyLogin {
		logger.Debugf("controller login: user %s has %q access", userTag.Id(), controllerAccess)
	} else {
		logger.Debugf("model login: user %s has %q for controller; %q for model %s",
			userTag.Id(), controllerAccess, modelAccess, a.root.model.ModelTag().Id())
	}
	return &params.AuthUserInfo{
		Identity:         userTag.String(),
		ControllerAccess: string(controllerAccess),
		ModelAccess:      string(modelAccess),
	}, nil
}

type facadeFilterFunc func(name string) bool

func filterFacades(registry *facade.Registry, allowFacadeAllMustMatch ...facadeFilterFunc) []params.FacadeVersions {
	allFacades := DescribeFacades(registry)
	out := make([]params.FacadeVersions, 0, len(allFacades))
	for _, f := range allFacades {
		allowed := false
		for _, allowFacade := range allowFacadeAllMustMatch {
			if allowed = allowFacade(f.Name); !allowed {
				break
			}
		}
		if allowed {
			out = append(out, f)
		}
	}
	return out
}

func (a *admin) maintenanceInProgress() bool {
	if !a.srv.upgradeComplete() {
		return true
	}
	switch a.srv.restoreStatus() {
	case state.RestorePending, state.RestoreInProgress:
		return true
	}
	return false
}

func setupPingTimeoutDisconnect(clock clock.Clock, root *apiHandler, entity state.Entity) error {
	tag := entity.Tag()
	if tag.Kind() == names.UserTagKind {
		return nil
	}

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
	pingTimeout := newPingTimeout(action, clock, maxClientPingInterval)
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
