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
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/pinger"
	"github.com/juju/juju/core/trace"
	jujuversion "github.com/juju/juju/core/version"
	accesserrors "github.com/juju/juju/domain/access/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/rpcreflect"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
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

	migrationMode, modelExists, err := a.getModelMigrationDetails(ctx, req)
	if err != nil {
		return fail, errors.Trace(err)
	}

	authResult, err := a.authenticate(ctx, modelExists, req)
	if err, ok := errors.Cause(err).(*apiservererrors.DischargeRequiredError); ok {
		loginResult := params.LoginResult{
			DischargeRequired:       err.LegacyMacaroon,
			BakeryDischargeRequired: err.Macaroon,
			DischargeRequiredReason: err.Error(),
		}
		logger.Infof(ctx, "login failed with discharge-required error: %v", err)
		return loginResult, nil
	}
	if err != nil {
		return fail, errors.Trace(err)
	}

	// Fetch the API server addresses from state.
	// If the login comes from a client, return all available addresses.
	// Otherwise return the addresses suitable for agent use.
	controllerNodeService := a.root.DomainServices().ControllerNode()
	getHostPorts := controllerNodeService.GetAPIHostPortsForAgents
	if k, _ := names.TagKind(req.AuthTag); k == names.UserTagKind {
		getHostPorts = controllerNodeService.GetAPIHostPortsForClients
	}
	hostPorts, err := getHostPorts(ctx)
	if err != nil {
		return fail, errors.Trace(err)
	}
	pServers := make([]network.HostPorts, len(hostPorts))
	for i, hps := range hostPorts {
		pServers[i] = hps
	}

	// apiRoot is the API root exposed to the client after login.
	var apiRoot rpc.Root
	apiRoot, err = newAPIRoot(
		a.root,
		a.srv.facades,
		httpRequestRecorderWrapper{
			collector: a.srv.metricsCollector,
			modelUUID: a.root.modelUUID,
		},
		a.srv.clock,
	)
	if err != nil {
		return fail, errors.Trace(err)
	}

	modelInfo, err := a.root.domainServices.Model().Model(ctx, a.root.modelUUID)
	if errors.Is(err, modelerrors.NotFound) {
		return fail, errors.NotFoundf("model %q", a.root.modelUUID)
	}
	if err != nil {
		return fail, errors.Trace(err)
	}

	apiRoot, err = restrictAPIRoot(
		a.srv,
		apiRoot,
		migrationMode,
		modelInfo.ModelType,
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
		modelTag = names.NewModelTag(a.root.modelUUID.String()).String()
	}

	auditConfig := a.srv.GetAuditConfig()
	auditRecorder, err := a.getAuditRecorder(ctx, req, modelInfo.Name, authResult, auditConfig)
	if err != nil {
		return fail, errors.Trace(err)
	}

	recorderFactory := observer.NewRecorderFactory(
		a.apiObserver, auditRecorder, auditConfig.CaptureAPIArgs,
	)
	a.root.rpcConn.ServeRoot(apiRoot, recorderFactory, serverError)
	return params.LoginResult{
		Servers:       params.FromHostsPorts(pServers),
		ControllerTag: names.NewControllerTag(a.srv.shared.controllerUUID).String(),
		UserInfo:      authResult.userInfo,
		ServerVersion: jujuversion.Current.String(),
		PublicDNSName: a.srv.publicDNSName(),
		ModelTag:      modelTag,
		Facades:       filterFacades(a.srv.facades, facadeFilters...),
	}, nil
}

func (a *admin) getAuditRecorder(
	ctx context.Context, req params.LoginRequest, modelName string, authResult *authResult, cfg auditlog.Config,
) (*auditlog.Recorder, error) {
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
			Who:          a.root.authInfo.Tag.Id(),
			What:         req.CLIArgs,
			ModelName:    modelName,
			ModelUUID:    a.root.modelUUID.String(),
			ConnectionID: a.root.connectionID,
		},
	)
	if err != nil {
		logger.Errorf(ctx, "couldn't add login to audit log: %+v", err)
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

func (a *admin) authenticate(ctx context.Context, modelExists bool, req params.LoginRequest) (*authResult, error) {
	result := &authResult{
		controllerOnlyLogin: a.root.controllerOnlyLogin,
		userLogin:           true,
	}

	logger.Debugf(ctx, "request authToken: %q", req.Token)
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
				logger.Tracef(ctx, "rate limiting for agent %s", req.AuthTag)
				return nil, errors.Trace(err)
			}
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
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

			authInfo, err = authenticator.AuthenticateLoginRequest(ctx, a.root.serverHost, a.root.modelUUID, authParams)
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

		isController, err := a.root.domainServices.ModelInfo().IsControllerModel(ctx)
		if errors.Is(err, modelerrors.NotFound) {
			return nil, errors.NotFoundf("model")
		}
		if err != nil {
			return nil, errors.Trace(err)
		}

		if result.controllerMachineLogin && !isController {
			// We only need to run a pinger for controller machine
			// agents when logging into the controller model.
			startPinger = false
			controllerConn = true
		}
	}
	if !modelExists {
		// Login to an unknown or migrated model.
		// See maybeEmitRedirectError for user logins who are redirected.
		// Hide the fact that the model does not exist.
		return nil, errors.Unauthorizedf("invalid entity name or password")
	}
	// TODO(wallyworld) - we can't yet observe anonymous logins as entity must be non-nil
	if !result.anonymousLogin {
		tag := names.NewModelTag(a.root.modelUUID.String())
		a.apiObserver.Login(ctx, a.root.authInfo.Tag, tag, a.root.modelUUID, controllerConn, req.UserData)
	}
	a.loggedIn = true

	if startPinger {
		if err := setupPingTimeoutDisconnect(ctx, a.srv.pingClock, a.root, a.root.authInfo.Tag); err != nil {
			return nil, errors.Trace(err)
		}
	}

	var lastConnection *time.Time
	if err := a.fillLoginDetails(ctx, authInfo, result, lastConnection); err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

func (a *admin) getModelMigrationDetails(ctx context.Context, req params.LoginRequest) (modelmigration.MigrationMode, bool, error) {
	// If the login attempt is by a user for a migrated model,
	// return a redirect error.
	// TODO - we'd want to use the model service here but migration
	//  artefacts still live in mongo.
	//  Ultimately we'd want a domain service API returning just:
	//    - model type
	//    - model name
	//    - migration mode

	exists, err := a.root.domainServices.Model().CheckModelExists(ctx, a.root.modelUUID)
	if err != nil {
		return "", false, errors.Trace(err)
	} else if !exists {
		err := a.maybeEmitRedirectError(ctx, req)
		return "", false, errors.Trace(err)
	}

	migrationMode, err := a.root.domainServices.ModelMigration().ModelMigrationMode(ctx)
	if err != nil {
		return "", false, errors.Trace(err)
	}

	return migrationMode, true, nil
}

func (a *admin) maybeEmitRedirectError(ctx context.Context, req params.LoginRequest) error {
	// Only need to redirect for user logins.
	if req.AuthTag == "" {
		return nil
	}
	authTag, err := names.ParseTag(req.AuthTag)
	if err != nil {
		return errors.Trace(err)
	}
	if authTag.Kind() != names.UserTagKind {
		return nil
	}

	// Check if the model was not found due to being migrated to another
	// controller.
	redirectionTarget, err := a.root.domainServices.Model().ModelRedirection(ctx, a.root.modelUUID)
	if errors.Is(err, modelerrors.ModelNotRedirected) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// If a user is trying to access a migrated model to which they are not
	// granted access, do not return a redirect error.
	// We need to return redirects if possible for anonymous logins in order
	// to ensure post-migration operation of CMRs.
	// TODO(aflynn): reinstate check for unauthorised user (JUJU-6669).

	hps, err := network.ParseProviderHostPorts(redirectionTarget.Addresses...)
	if err != nil {
		return errors.Trace(err)
	}

	return &apiservererrors.RedirectError{
		Servers:         []network.ProviderHostPorts{hps},
		CACert:          redirectionTarget.CACert,
		ControllerTag:   names.NewControllerTag(redirectionTarget.ControllerUUID),
		ControllerAlias: redirectionTarget.ControllerAlias,
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
			logger.Debugf(ctx, " anonymous controller login")
		} else {
			logger.Debugf(ctx, "controller login: %s", a.root.authInfo.Tag)
		}
	} else {
		if result.anonymousLogin {
			logger.Debugf(ctx, "anonymous model login")
		} else {
			logger.Debugf(ctx, "model login: %s for model %s", a.root.authInfo.Tag, a.root.modelUUID)
		}
	}
	return nil
}

func (a *admin) checkUserPermissions(
	ctx context.Context,
	authInfo authentication.AuthInfo,
	controllerOnlyLogin bool,
) (*params.AuthUserInfo, error) {
	userTag, ok := authInfo.Tag.(names.UserTag)
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
			Key:        a.root.modelUUID.String(),
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
		logger.Debugf(ctx, "controller login: user %s has %q access", userTag.Id(), controllerAccess)
	} else {
		logger.Debugf(ctx, "model login: user %s has %q for controller; %q for model %s",
			userTag.Id(), controllerAccess, modelAccess, a.root.modelUUID)
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

func setupPingTimeoutDisconnect(ctx context.Context, clock clock.Clock, root PingRootHandler, tag names.Tag) error {
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
		logger.Debugf(ctx, "closing connection due to ping timeout")
		if err := root.CloseConn(); err != nil {
			logger.Errorf(ctx, "error closing the RPC connection: %v", err)
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
