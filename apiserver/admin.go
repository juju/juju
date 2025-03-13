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
	"github.com/juju/names/v6"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/pinger"
	"github.com/juju/juju/core/trace"
	jujuversion "github.com/juju/juju/core/version"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/internal/rpcreflect"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
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
		return nil, apiservererrors.ErrBadId
	}
	return a, nil
}

// Login logs in with the provided credentials.  All subsequent requests on the
// connection will act as the authenticated user.
func (a *admin) Login(ctx context.Context, req params.LoginRequest) (params.LoginResult, error) {
	return a.login(ctx, req, 3)
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
	if err, ok := errors.Cause(err).(*apiservererrors.DischargeRequiredError); ok {
		loginResult := params.LoginResult{
			DischargeRequired:       err.LegacyMacaroon,
			BakeryDischargeRequired: err.Macaroon,
			DischargeRequiredReason: err.Error(),
		}
		logger.Infof(context.TODO(), "login failed with discharge-required error: %v", err)
		return loginResult, nil
	}
	if err != nil {
		return fail, errors.Trace(err)
	}

	controllerConfigService := a.root.DomainServices().ControllerConfig()
	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return fail, errors.Trace(err)
	}

	// Fetch the API server addresses from state.
	// If the login comes from a client, return all available addresses.
	// Otherwise return the addresses suitable for agent use.
	ctrlSt, err := a.root.shared.statePool.SystemState()
	if err != nil {
		return fail, errors.Trace(err)
	}
	getHostPorts := ctrlSt.APIHostPortsForAgents
	if k, _ := names.TagKind(req.AuthTag); k == names.UserTagKind {
		getHostPorts = ctrlSt.APIHostPortsForClients
	}
	hostPorts, err := getHostPorts(controllerConfig)
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
		a.root,
		a.srv.facades,
		httpRequestRecorderWrapper{
			collector: a.srv.metricsCollector,
			modelUUID: a.root.model.UUID(),
		},
		a.srv.clock,
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
			Who:          a.root.authInfo.Entity.Tag().Id(),
			What:         req.CLIArgs,
			ModelName:    a.root.model.Name(),
			ModelUUID:    a.root.model.UUID(),
			ConnectionID: a.root.connectionID,
		},
	)
	if err != nil {
		logger.Errorf(context.TODO(), "couldn't add login to audit log: %+v", err)
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
		controllerOnlyLogin: a.root.controllerOnlyLogin,
		userLogin:           true,
	}

	logger.Debugf(context.TODO(), "request authToken: %q", req.Token)
	if req.Token == "" && req.AuthTag != "" {
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
				logger.Tracef(context.TODO(), "rate limiting for agent %s", req.AuthTag)
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
		modelUUID = model.UUID(a.root.model.UUID())
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

	var authInfo authentication.AuthInfo

	// controllerConn is used to indicate a connection from
	// the controller to a non-controller model.
	controllerConn := false
	if !result.anonymousLogin {
		authParams := authentication.AuthParams{
			AuthTag:       result.tag,
			Credentials:   req.Credentials,
			Nonce:         req.Nonce,
			Token:         req.Token,
			Macaroons:     req.Macaroons,
			BakeryVersion: req.BakeryVersion,
		}

		authenticated := false
		for _, authenticator := range a.srv.loginAuthenticators {
			var err error

			authInfo, err = authenticator.AuthenticateLoginRequest(ctx, a.root.serverHost, modelUUID, authParams)
			if errors.Is(err, errors.NotSupported) {
				continue
			} else if errors.Is(err, errors.NotImplemented) {
				continue
			} else if err != nil {
				return nil, a.handleAuthError(err)
			}

			authenticated = true
			a.root.authInfo = authInfo
			result.controllerMachineLogin = authInfo.Controller
			break
		}

		if !authenticated {
			return nil, fmt.Errorf("failed to authenticate request: %w", errors.Unauthorized)
		}

		if result.controllerMachineLogin && !a.root.state.IsController() {
			// We only need to run a pinger for controller machine
			// agents when logging into the controller model.
			startPinger = false
			controllerConn = true
		}
	}
	if a.root.model == nil {
		// Login to an unknown or migrated model.
		// See maybeEmitRedirectError for user logins who are redirected.
		// Hide the fact that the model does not exist.
		return nil, errors.Unauthorizedf("invalid entity name or password")
	}
	// TODO(wallyworld) - we can't yet observe anonymous logins as entity must be non-nil
	if !result.anonymousLogin {
		a.apiObserver.Login(ctx, a.root.authInfo.Entity.Tag(), a.root.model.ModelTag(), a.root.modelUUID, controllerConn, req.UserData)
	}
	a.loggedIn = true

	if startPinger {
		if err := setupPingTimeoutDisconnect(a.srv.pingClock, a.root, a.root.authInfo.Entity); err != nil {
			return nil, errors.Trace(err)
		}
	}

	var lastConnection *time.Time
	if err := a.fillLoginDetails(ctx, authInfo, result, lastConnection); err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

func (a *admin) maybeEmitRedirectError(modelUUID model.UUID, authTag names.Tag) error {
	_, ok := authTag.(names.UserTag)
	if !ok {
		return nil
	}

	st, err := a.root.shared.statePool.Get(modelUUID.String())
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = st.Release() }()

	// If the model exists on this controller then no redirect is possible.
	if _, err := st.Model(); err == nil || !errors.Is(err, errors.NotFound) {
		return nil
	}

	// Check if the model was not found due to
	// being migrated to another controller.
	mig, err := st.CompletedMigration()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}

	// If a user is trying to access a migrated model to which they are not
	// granted access, do not return a redirect error.
	// We need to return redirects if possible for anonymous logins in order
	// to ensure post-migration operation of CMRs.
	// TODO(aflynn): reinstate check for unauthorised user (JUJU-6669).
	if mig == nil {
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

	return &apiservererrors.RedirectError{
		Servers:         []network.ProviderHostPorts{hps},
		CACert:          target.CACert,
		ControllerTag:   target.ControllerTag,
		ControllerAlias: target.ControllerAlias,
	}
}

func (a *admin) handleAuthError(err error) error {
	if err, ok := errors.Cause(err).(*apiservererrors.DischargeRequiredError); ok {
		return err
	}
	if !a.srv.upgradeComplete() {
		// An upgrade, migration or similar operation is in
		// progress. It is possible for logins to fail until this
		// is complete due to incomplete or updating data. Mask
		// transitory and potentially confusing errors from failed
		// logins with a more helpful one.
		return errors.Wrap(err, MaintenanceNoLoginError)
	}
	return err
}

func (a *admin) fillLoginDetails(ctx context.Context, authInfo authentication.AuthInfo, result *authResult, lastConnection *time.Time) error {
	// Send back user info if user
	if result.userLogin {
		var err error
		result.userInfo, err = a.checkUserPermissions(ctx, authInfo, result.controllerOnlyLogin)
		if err != nil {
			return errors.Trace(err)
		}
		result.userInfo.LastConnection = lastConnection
	}
	if result.controllerOnlyLogin {
		if result.anonymousLogin {
			logger.Debugf(context.TODO(), " anonymous controller login")
		} else {
			logger.Debugf(context.TODO(), "controller login: %s", a.root.authInfo.Entity.Tag())
		}
	} else {
		if result.anonymousLogin {
			logger.Debugf(context.TODO(), "anonymous model login")
		} else {
			logger.Debugf(context.TODO(), "model login: %s for %s", a.root.authInfo.Entity.Tag(), a.root.model.ModelTag().Id())
		}
	}
	return nil
}

func (a *admin) checkUserPermissions(
	ctx context.Context,
	authInfo authentication.AuthInfo,
	controllerOnlyLogin bool,
) (*params.AuthUserInfo, error) {
	userTag, ok := authInfo.Entity.Tag().(names.UserTag)
	if !ok {
		return nil, fmt.Errorf("establishing user tag from authenticated user entity")
	}

	controllerAccess, err := authInfo.SubjectPermissions(ctx, permission.ID{
		ObjectType: permission.Controller,
		Key:        a.srv.shared.controllerUUID,
	})
	if errors.Is(err, accesserrors.PermissionNotFound) || errors.Is(err, accesserrors.UserNotFound) {
		controllerAccess = permission.NoAccess
	} else if err != nil {
		return nil, errors.Annotatef(err, "obtaining ControllerUser for logged in user %s", userTag.Id())
	}

	modelAccess := permission.NoAccess
	if !controllerOnlyLogin {
		// Only grab modelUser permissions if this is not a controller only
		// login. In all situations, if the model user is not found, they have
		// no authorisation to access this model, unless the user is controller
		// admin.

		var err error
		modelAccess, err = authInfo.SubjectPermissions(ctx, permission.ID{
			ObjectType: permission.Model,
			Key:        a.root.model.ModelTag().Id(),
		})
		if err != nil {
			if controllerAccess != permission.SuperuserAccess {
				return nil, errors.Wrap(err, apiservererrors.ErrPerm)
			}
			modelAccess = permission.AdminAccess
		}
	}

	if controllerOnlyLogin || !a.srv.allowModelAccess {
		// We're either explicitly logging into the controller or
		// we must check that the user has access to the controller
		// even though they're logging into a model.
		if controllerAccess == permission.NoAccess {
			return nil, errors.Trace(apiservererrors.ErrPerm)
		}
	}
	if controllerOnlyLogin {
		logger.Debugf(context.TODO(), "controller login: user %s has %q access", userTag.Id(), controllerAccess)
	} else {
		logger.Debugf(context.TODO(), "model login: user %s has %q for controller; %q for model %s",
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

// PingRootHandler is the interface that the root handler must implement
// to allow the pinger to be registered.
type PingRootHandler interface {
	WatcherRegistry() facade.WatcherRegistry
	CloseConn() error
}

func setupPingTimeoutDisconnect(clock clock.Clock, root PingRootHandler, entity state.Entity) error {
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
		logger.Debugf(context.TODO(), "closing connection due to ping timeout")
		if err := root.CloseConn(); err != nil {
			logger.Errorf(context.TODO(), "error closing the RPC connection: %v", err)
		}
	}
	p := pinger.NewPinger(action, clock, maxClientPingInterval)
	return root.WatcherRegistry().RegisterNamed("pingTimeout", p)
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

// StartTrace returns a noop span, we probably still want to enable tracing
// even in this state. For now, we'll just return a noop span.
// TODO(stickupkid): Revisit this when we understand this path better.
func (r *errRoot) StartTrace(ctx context.Context) (context.Context, trace.Span) {
	return ctx, trace.NoopSpan{}
}

func (r *errRoot) Kill() {}
