// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher/registry"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/rpcreflect"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
)

type objectKey struct {
	name    string
	version int
	objId   string
}

// apiHandler represents a single client's connection to the state
// after it has logged in. It contains an rpc.Root which it
// uses to dispatch API calls appropriately.
type apiHandler struct {
	rpcConn *rpc.Conn

	// TODO (stickupkid): The "shared" concept is an abomination, we should
	// remove this and pass the dependencies in directly.
	shared *sharedServerContext

	// tracer is the tracing worker (OTEL) for the resolved model UUID. This
	// is either the request model UUID, or it's the system state model UUID, if
	// the request model UUID is empty.
	tracer trace.Tracer

	// domainServices is the domain services for the resolved model UUID. This
	// is either the request model UUID, or it's the system state model UUID, if
	// the request model UUID is empty.
	domainServices services.DomainServices

	// domainServicesGetter allows the retrieval of an domain services for a
	// given model UUID. This should not be used unless you're sure you need to
	// access a different model's domain services.
	domainServicesGetter services.DomainServicesGetter

	// objectStore is the object store for the resolved model UUID. This is
	// either the request model UUID, or it's the system state model UUID, if
	// the request model UUID is empty.
	objectStore objectstore.ObjectStore

	// objectStoreGetter allows the retrieval of an object store for a given
	// model UUID. This should not be used unless you're sure you need to
	// access a different model's object store.
	objectStoreGetter objectstore.ObjectStoreGetter

	// controllerObjectStore is the object store for the controller namespace.
	// This is the global namespace and is used for agent binaries and other
	// controller-wide binary data.
	controllerObjectStore objectstore.ObjectStore

	// watcherRegistry is the registry for tracking watchers between API calls
	// for a given model UUID.
	watcherRegistry facade.WatcherRegistry

	// authInfo represents the authentication info established with this client
	// connection.
	authInfo authentication.AuthInfo

	// modelUUID is the UUID of the model that the client is connected to.
	// All facades for a given context will be scoped to the model UUID.
	// Facade methods should only scoped to the model UUID they are operating
	// on. There are some exceptions to this rule, but they are exceptions that
	// prove the rule.
	modelUUID model.UUID

	// controllerOnlyLogin is true if the client is using controller
	// routes. Ultimately, this just indicates that no model UUID was specified
	// in the request query (:modeluuid).
	controllerOnlyLogin bool

	// connectionID is shared between the API observer (including API
	// requests and responses in the agent log) and the audit logger.
	connectionID uint64

	// serverHost is the host:port of the API server that the client
	// connected to.
	serverHost string
}

var _ = (*apiHandler)(nil)

var (
	// maxClientPingInterval defines the timeframe until the ping timeout
	// closes the monitored connection. TODO(mue): Idea by Roger:
	// Move to API (e.g. params) so that the pinging there may
	// depend on the interval.
	maxClientPingInterval = 3 * time.Minute
)

// newAPIHandler returns a new apiHandler.
func newAPIHandler(
	ctx context.Context,
	srv *Server,
	rpcConn *rpc.Conn,
	domainServices services.DomainServices,
	domainServicesGetter services.DomainServicesGetter,
	tracer trace.Tracer,
	objectStore objectstore.ObjectStore,
	objectStoreGetter objectstore.ObjectStoreGetter,
	controllerObjectStore objectstore.ObjectStore,
	modelUUID model.UUID,
	controllerOnlyLogin bool,
	connectionID uint64,
	serverHost string,
) (*apiHandler, error) {
	exists, err := domainServices.Model().CheckModelExists(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !exists {
		// If this model used to be hosted on this controller but got
		// migrated allow clients to connect and wait for a login
		// request to decide whether the users should be redirected to
		// the new controller for this model or not.
		if _, migErr := domainServices.Model().ModelRedirection(ctx, modelUUID); migErr != nil {
			// Return not found on any error.
			// TODO (stickupkid): This is very brute force. What if there
			// is an error with the database? The caller will assume that it
			// is no longer on this controller. If we return a different error
			// then it can at least retry the request.
			return nil, errors.NotFoundf("model %q", modelUUID)
		}
	}

	registry, err := registry.NewRegistry(srv.clock, registry.WithLogger(logger.Child("registry", corelogger.WATCHERS)))
	if err != nil {
		return nil, errors.Trace(err)
	}

	r := &apiHandler{
		domainServices:        domainServices,
		domainServicesGetter:  domainServicesGetter,
		tracer:                tracer,
		objectStore:           objectStore,
		objectStoreGetter:     objectStoreGetter,
		controllerObjectStore: controllerObjectStore,
		watcherRegistry:       registry,
		shared:                srv.shared,
		rpcConn:               rpcConn,
		modelUUID:             modelUUID,
		controllerOnlyLogin:   controllerOnlyLogin,
		connectionID:          connectionID,
		serverHost:            serverHost,
	}

	return r, nil
}

// WatcherRegistry returns the watcher registry for tracking watchers between
// API calls.
func (r *apiHandler) WatcherRegistry() facade.WatcherRegistry {
	return r.watcherRegistry
}

// DomainServices returns the domain services.
func (r *apiHandler) DomainServices() services.DomainServices {
	return r.domainServices
}

// DomainServicesGetter returns the domain services getter.
func (r *apiHandler) DomainServicesGetter() services.DomainServicesGetter {
	return r.domainServicesGetter
}

// Tracer returns the tracer for opentelemetry.
func (r *apiHandler) Tracer() trace.Tracer {
	return r.tracer
}

// ObjectStore returns the object store.
func (r *apiHandler) ObjectStore() objectstore.ObjectStore {
	return r.objectStore
}

// ObjectStoreGetter returns the object store getter.
func (r *apiHandler) ObjectStoreGetter() objectstore.ObjectStoreGetter {
	return r.objectStoreGetter
}

// ControllerObjectStore returns the controller object store. The primary
// use case for this is agent tools.
func (r *apiHandler) ControllerObjectStore() objectstore.ObjectStore {
	return r.controllerObjectStore
}

// SharedContext returns the server shared context.
func (r *apiHandler) SharedContext() *sharedServerContext {
	return r.shared
}

// Authorizer returns the authorizer used for accessing API method calls.
func (r *apiHandler) Authorizer() facade.Authorizer {
	return r
}

// ModelUUID returns the UUID of the model that the API is operating on.
func (r *apiHandler) ModelUUID() model.UUID {
	return r.modelUUID
}

// CloseConn closes the underlying connection.
func (r *apiHandler) CloseConn() error {
	return r.rpcConn.Close()
}

// Kill implements rpc.Killer, cleaning up any resources that need
// cleaning up to ensure that all outstanding requests return.
func (r *apiHandler) Kill() {
	r.watcherRegistry.Kill()
	if err := r.watcherRegistry.Wait(); err != nil {
		logger.Infof(context.TODO(), "error waiting for watcher registry to stop: %v", err)
	}
}

// AuthMachineAgent returns whether the current client is a machine agent.
// TODO(controlleragent) - add AuthControllerAgent function
func (r *apiHandler) AuthMachineAgent() bool {
	_, isMachine := r.GetAuthTag().(names.MachineTag)
	_, isControllerAgent := r.GetAuthTag().(names.ControllerAgentTag)
	return isMachine || isControllerAgent
}

// AuthModelAgent return whether the current client is a model agent
func (r *apiHandler) AuthModelAgent() bool {
	_, isModel := r.GetAuthTag().(names.ModelTag)
	return isModel
}

// AuthApplicationAgent returns whether the current client is an application operator.
func (r *apiHandler) AuthApplicationAgent() bool {
	_, isApp := r.GetAuthTag().(names.ApplicationTag)
	return isApp
}

// AuthUnitAgent returns whether the current client is a unit agent.
func (r *apiHandler) AuthUnitAgent() bool {
	_, isUnit := r.GetAuthTag().(names.UnitTag)
	return isUnit
}

// AuthOwner returns whether the authenticated user's tag matches the
// given entity tag.
func (r *apiHandler) AuthOwner(tag names.Tag) bool {
	return r.GetAuthTag() == tag
}

// AuthController returns whether the authenticated user is a
// machine with running the ManageEnviron job.
func (r *apiHandler) AuthController() bool {
	return r.authInfo.Controller
}

// AuthClient returns whether the authenticated entity is a client
// user.
func (r *apiHandler) AuthClient() bool {
	_, isUser := r.GetAuthTag().(names.UserTag)
	return isUser
}

// GetAuthTag returns the tag of the authenticated entity, if any.
func (r *apiHandler) GetAuthTag() names.Tag {
	return r.authInfo.Tag
}

// HasPermission is responsible for reporting if the logged in user is
// able to perform operation x on target y. It uses the authentication mechanism
// of the user to interrogate their permissions. If the entity does not have
// permission to perform the operation then the authentication provider is asked
// to provide a permission error. All permissions errors returned satisfy
// errors.Is(err, ErrorEntityMissingPermission) to distinguish before errors and
// no permissions errors. If error is nil then the user has permission.
func (r *apiHandler) HasPermission(ctx context.Context, operation permission.Access, target names.Tag) error {
	return r.EntityHasPermission(ctx, r.GetAuthTag(), operation, target)
}

// EntityHasPermission is responsible for reporting if the supplied entity is
// able to perform operation x on target y. It uses the authentication mechanism
// of the user to interrogate their permissions. If the entity does not have
// permission to perform the operation then the authentication provider is asked
// to provide a permission error. All permissions errors returned satisfy
// errors.Is(err, ErrorEntityMissingPermission) to distinguish before errors and
// no permissions errors. If error is nil then the user has permission.
func (r *apiHandler) EntityHasPermission(
	ctx context.Context, entity names.Tag, operation permission.Access, target names.Tag,
) error {
	var userAccessFunc common.UserAccessFunc = func(ctx context.Context, userName user.Name, target permission.ID) (permission.Access, error) {
		if r.authInfo.Delegator == nil {
			return permission.NoAccess, fmt.Errorf("permissions %w for auth info", errors.NotImplemented)
		}
		return r.authInfo.Delegator.SubjectPermissions(ctx, userName.Name(), target)
	}
	has, err := common.HasPermission(ctx, userAccessFunc, entity, operation, target)
	if err != nil {
		return fmt.Errorf("checking entity %q has permission: %w", entity, err)
	}
	if !has && r.authInfo.Delegator != nil {
		err = r.authInfo.Delegator.PermissionError(target, operation)
	}
	if !has {
		return errors.WithType(err, authentication.ErrorEntityMissingPermission)
	}

	return nil
}

// srvCaller is our implementation of the rpcreflect.MethodCaller interface.
// It lives just long enough to encapsulate the methods that should be
// available for an RPC call and allow the RPC code to instantiate an object
// and place a call on its method.
type srvCaller struct {
	objMethod rpcreflect.ObjMethod
	creator   func(ctx context.Context, id string) (reflect.Value, error)
}

// ParamsType defines the parameters that should be supplied to this function.
// See rpcreflect.MethodCaller for more detail.
func (s *srvCaller) ParamsType() reflect.Type {
	return s.objMethod.Params
}

// ResultType defines the object that is returned from the function.`
// See rpcreflect.MethodCaller for more detail.
func (s *srvCaller) ResultType() reflect.Type {
	return s.objMethod.Result
}

// Call takes the object Id and an instance of ParamsType to create an object and place
// a call on its method. It then returns an instance of ResultType.
func (s *srvCaller) Call(ctx context.Context, objId string, arg reflect.Value) (reflect.Value, error) {
	objVal, err := s.creator(ctx, objId)
	if err != nil {
		return reflect.Value{}, err
	}
	return s.objMethod.Call(ctx, objVal, arg)
}

type apiRootHandler interface {
	rpc.Killer
	// DomainServices returns the domain services.
	DomainServices() services.DomainServices
	// DomainServicesGetter returns the domain services getter.
	DomainServicesGetter() services.DomainServicesGetter
	// Tracer returns the tracer for opentelemetry.
	Tracer() trace.Tracer
	// ObjectStore returns the object store.
	ObjectStore() objectstore.ObjectStore
	// ObjectStoreGetter returns the object store getter.
	ObjectStoreGetter() objectstore.ObjectStoreGetter
	// ControllerObjectStore returns the controller object store. The primary
	// use case for this is agent tools.
	ControllerObjectStore() objectstore.ObjectStore
	// SharedContext returns the server shared context.
	SharedContext() *sharedServerContext
	// WatcherRegistry returns the watcher registry for tracking watchers
	// between API calls.
	WatcherRegistry() facade.WatcherRegistry
	// Authorizer returns the authorizer used for accessing API method calls.
	Authorizer() facade.Authorizer
	// ModelUUID returns the UUID of the model that the API is operating on.
	ModelUUID() model.UUID
}

// apiRoot implements basic method dispatching to the facade registry.
type apiRoot struct {
	rpc.Killer
	clock                 clock.Clock
	domainServices        services.DomainServices
	domainServicesGetter  services.DomainServicesGetter
	tracer                trace.Tracer
	objectStore           objectstore.ObjectStore
	objectStoreGetter     objectstore.ObjectStoreGetter
	controllerObjectStore objectstore.ObjectStore
	shared                *sharedServerContext
	facades               *facade.Registry
	watcherRegistry       facade.WatcherRegistry
	authorizer            facade.Authorizer
	objectMutex           sync.RWMutex
	objectCache           map[objectKey]reflect.Value
	requestRecorder       facade.RequestRecorder

	// modelUUID is the UUID of the model that the client is connected to.
	// All facades for a given context will be scoped to the model UUID.
	// Facade methods should only scoped to the model UUID they are operating
	// on. There are some exceptions to this rule, but they are exceptions that
	// prove the rule.
	modelUUID model.UUID
}

// newAPIRoot returns a new apiRoot.
func newAPIRoot(
	root apiRootHandler,
	facades *facade.Registry,
	requestRecorder facade.RequestRecorder,
	clock clock.Clock,
) (*apiRoot, error) {
	return &apiRoot{
		Killer:                root,
		clock:                 clock,
		domainServices:        root.DomainServices(),
		domainServicesGetter:  root.DomainServicesGetter(),
		tracer:                root.Tracer(),
		objectStore:           root.ObjectStore(),
		objectStoreGetter:     root.ObjectStoreGetter(),
		controllerObjectStore: root.ControllerObjectStore(),
		shared:                root.SharedContext(),
		facades:               facades,
		watcherRegistry:       root.WatcherRegistry(),
		authorizer:            root.Authorizer(),
		objectCache:           make(map[objectKey]reflect.Value),
		requestRecorder:       requestRecorder,
		modelUUID:             root.ModelUUID(),
	}, nil
}

// restrictAPIRoot calls restrictAPIRootDuringMaintenance, and
// then restricts the result further to the controller or model
// facades, depending on the type of login.
func restrictAPIRoot(
	srv *Server,
	apiRoot rpc.Root,
	migrationMode modelmigration.MigrationMode,
	modelType model.ModelType,
	auth authResult,
) (rpc.Root, error) {
	if !auth.controllerMachineLogin {
		// Controller agents are allowed to
		// connect even during maintenance.
		restrictedRoot, err := restrictAPIRootDuringMaintenance(
			srv, apiRoot, migrationMode, auth.tag,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		apiRoot = restrictedRoot
	}
	if auth.controllerOnlyLogin {
		apiRoot = restrictRoot(apiRoot, controllerFacadesOnly)
	} else {
		apiRoot = restrictRoot(apiRoot, modelFacadesOnly)
		if modelType == model.CAAS {
			apiRoot = restrictRoot(apiRoot, caasModelFacadesOnly)
		}
	}
	return apiRoot, nil
}

// restrictAPIRootDuringMaintenance restricts the API root during
// maintenance events (upgrade or migration), depending
// on the authenticated client.
func restrictAPIRootDuringMaintenance(
	srv *Server,
	apiRoot rpc.Root,
	migrationMode modelmigration.MigrationMode,
	authTag names.Tag,
) (rpc.Root, error) {
	describeLogin := func() string {
		if authTag == nil {
			return "anonymous login"
		}
		return fmt.Sprintf("login for %s", names.ReadableString(authTag))
	}

	if !srv.upgradeComplete() {
		if _, ok := authTag.(names.UserTag); ok {
			// Users get access to a limited set of functionality
			// while an upgrade is in progress.
			return restrictRoot(apiRoot, upgradeMethodsOnly), nil
		}
		// Agent and anonymous logins are blocked during upgrade.
		return nil, errors.Errorf("%s blocked because upgrade is in progress", describeLogin())
	}

	// For user logins, we limit access during migrations.
	if _, ok := authTag.(names.UserTag); ok {
		switch migrationMode {
		case modelmigration.MigrationModeImporting:
			// The user is not able to access a model that is currently being
			// imported until the model has been activated.
			apiRoot = restrictAll(apiRoot, errors.New("migration in progress, model is importing"))
		case modelmigration.MigrationModeExporting:
			// The user is not allowed to change anything in a model that is
			// currently being moved to another controller.
			apiRoot = restrictRoot(apiRoot, migrationClientMethodsOnly)
		}
	}

	return apiRoot, nil
}

// StartTrace starts a trace based on the underlying given context, that
// is in the context of the apiserver.
func (r *apiRoot) StartTrace(ctx context.Context) (context.Context, trace.Span) {
	ctx = trace.WithTracer(ctx, r.tracer)
	return trace.Start(ctx, trace.NameFromFunc())
}

// FindMethod looks up the given rootName and version in our facade registry
// and returns a MethodCaller that will be used by the RPC code to place calls on
// that facade.
// FindMethod uses the global registry apiserver/common.Facades.
// For more information about how FindMethod should work, see rpc/server.go and
// rpc/rpcreflect/value.go
func (r *apiRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	goType, objMethod, err := r.lookupMethod(rootName, version, methodName)
	if err != nil {
		return nil, err
	}

	creator := func(ctx context.Context, id string) (reflect.Value, error) {
		objKey := objectKey{name: rootName, version: version, objId: id}
		r.objectMutex.RLock()
		objValue, ok := r.objectCache[objKey]
		r.objectMutex.RUnlock()
		if ok {
			return objValue, nil
		}
		r.objectMutex.Lock()
		defer r.objectMutex.Unlock()
		if objValue, ok := r.objectCache[objKey]; ok {
			return objValue, nil
		}
		// Now that we have the write lock, check one more time in case
		// someone got the write lock before us.
		factory, err := r.facades.GetFactory(rootName, version)
		if err != nil {
			// We don't check for IsNotFound here, because it
			// should have already been handled in the GetType
			// check.
			return reflect.Value{}, err
		}
		obj, err := factory(ctx, r.facadeContext(objKey))
		if err != nil {
			return reflect.Value{}, err
		}
		objValue = reflect.ValueOf(obj)
		if !objValue.Type().AssignableTo(goType) {
			return reflect.Value{}, errors.Errorf(
				"internal error, %s(%d) claimed to return %s but returned %T",
				rootName, version, goType, obj)
		}
		if goType.Kind() == reflect.Interface {
			// If the original function wanted to return an
			// interface type, the indirection in the factory via
			// an interface{} strips the original interface
			// information off. So here we have to create the
			// interface again, and assign it.
			asInterface := reflect.New(goType).Elem()
			asInterface.Set(objValue)
			objValue = asInterface
		}
		r.objectCache[objKey] = objValue
		return objValue, nil
	}
	return &srvCaller{
		creator:   creator,
		objMethod: objMethod,
	}, nil
}

func (r *apiRoot) lookupMethod(rootName string, version int, methodName string) (reflect.Type, rpcreflect.ObjMethod, error) {
	goType, err := r.facades.GetType(rootName, version)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return nil, rpcreflect.ObjMethod{}, &rpcreflect.CallNotImplementedError{
				RootMethod: rootName,
				Version:    version,
			}
		}
		return nil, rpcreflect.ObjMethod{}, err
	}
	rpcType := rpcreflect.ObjTypeOf(goType)
	objMethod, err := rpcType.Method(methodName)
	if err != nil {
		if err == rpcreflect.ErrMethodNotFound {
			return nil, rpcreflect.ObjMethod{}, &rpcreflect.CallNotImplementedError{
				RootMethod: rootName,
				Version:    version,
				Method:     methodName,
			}
		}
		return nil, rpcreflect.ObjMethod{}, err
	}
	return goType, objMethod, nil
}

func (r *apiRoot) dispose(key objectKey) {
	r.objectMutex.Lock()
	defer r.objectMutex.Unlock()
	delete(r.objectCache, key)
}

func (r *apiRoot) facadeContext(key objectKey) *facadeContext {
	return &facadeContext{
		r:   r,
		key: key,
	}
}

// adminRoot dispatches API calls to those available to an anonymous connection
// which has not logged in, which here is the admin facade.
type adminRoot struct {
	*apiHandler
	reflectAPIs map[int]rpcreflect.Value
}

// newAdminRoot creates a new AnonRoot which dispatches to the given Admin API implementation.
func newAdminRoot(h *apiHandler, adminAPIs map[int]any) *adminRoot {
	reflects := make(map[int]rpcreflect.Value, len(adminAPIs))
	for version, api := range adminAPIs {
		reflects[version] = rpcreflect.ValueOf(reflect.ValueOf(api))
	}
	r := &adminRoot{
		apiHandler:  h,
		reflectAPIs: reflects,
	}
	return r
}

// StartTrace starts a trace based on the underlying given context, that
// is in the context of the apiserver.
func (r *adminRoot) StartTrace(ctx context.Context) (context.Context, trace.Span) {
	ctx = trace.WithTracer(ctx, r.tracer)
	return trace.Start(ctx, trace.NameFromFunc())
}

func (r *adminRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if rootName != "Admin" {
		return nil, &rpcreflect.CallNotImplementedError{
			RootMethod: rootName,
			Version:    version,
		}
	}
	if reflectAPI, ok := r.reflectAPIs[version]; ok {
		return reflectAPI.FindMethod(rootName, 0, methodName)
	}
	return nil, &rpc.RequestError{
		Code:    params.CodeNotSupported,
		Message: "this version of Juju does not support login from old clients",
	}
}

// facadeContext implements facade.ModelContext
type facadeContext struct {
	r   *apiRoot
	key objectKey
}

// Auth is part of the facade.ModelContext interface.
func (ctx *facadeContext) Auth() facade.Authorizer {
	return ctx.r.authorizer
}

// Dispose is part of the facade.ModelContext interface.
func (ctx *facadeContext) Dispose() {
	ctx.r.dispose(ctx.key)
}

// WatcherRegistry is part of the facade.ModelContext interface.
func (ctx *facadeContext) WatcherRegistry() facade.WatcherRegistry {
	return ctx.r.watcherRegistry
}

// ControllerUUID returns the controller unique identifier.
func (ctx *facadeContext) ControllerUUID() string {
	return ctx.r.shared.controllerUUID
}

// ControllerModelUUID returns the controller model unique identifier.
func (ctx *facadeContext) ControllerModelUUID() model.UUID {
	return ctx.r.shared.controllerModelUUID
}

// IsControllerModelScoped returns whether the context is scoped to the
// controller model.
func (ctx *facadeContext) IsControllerModelScoped() bool {
	return ctx.ModelUUID() == ctx.ControllerModelUUID()
}

// ModelUUID returns the model unique identifier.
func (ctx *facadeContext) ModelUUID() model.UUID {
	return ctx.r.modelUUID
}

// ID is part of the facade.ModelContext interface.
func (ctx *facadeContext) ID() string {
	return ctx.key.objId
}

// RequestRecorder defines a metrics collector for outbound requests.
func (ctx *facadeContext) RequestRecorder() facade.RequestRecorder {
	return ctx.r.requestRecorder
}

// LeadershipChecker is part of the facade.ModelContext interface.
func (ctx *facadeContext) LeadershipChecker() (leadership.Checker, error) {
	checker, err := ctx.r.shared.leaseManager.Checker(
		lease.ApplicationLeadershipNamespace,
		ctx.ModelUUID().String(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return leadershipChecker{checker: checker}, nil
}

// SingularClaimer is part of the facade.ModelContext interface.
func (ctx *facadeContext) SingularClaimer() (lease.Claimer, error) {
	return ctx.r.shared.leaseManager.Claimer(
		lease.SingularControllerNamespace,
		ctx.ModelUUID().String(),
	)
}

// LeadershipClaimer is part of the facade.ModelContext interface.
func (ctx *facadeContext) LeadershipClaimer() (leadership.Claimer, error) {
	claimer, err := ctx.r.shared.leaseManager.Claimer(
		lease.ApplicationLeadershipNamespace,
		ctx.ModelUUID().String(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return leadershipClaimer{claimer: claimer}, nil
}

// LeadershipRevoker is part of the facade.ModelContext interface.
func (ctx *facadeContext) LeadershipRevoker() (leadership.Revoker, error) {
	revoker, err := ctx.r.shared.leaseManager.Revoker(
		lease.ApplicationLeadershipNamespace,
		ctx.ModelUUID().String(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return leadershipRevoker{claimer: revoker}, nil
}

// LeadershipPinner is part of the facade.ModelContext interface.
// Pinning functionality is only available with the Raft leases implementation.
func (ctx *facadeContext) LeadershipPinner() (leadership.Pinner, error) {
	pinner, err := ctx.r.shared.leaseManager.Pinner(
		lease.ApplicationLeadershipNamespace,
		ctx.ModelUUID().String(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return leadershipPinner{pinner: pinner}, nil
}

// LeadershipReader is part of the facade.ModelContext interface.
// It returns a reader that can be used to return all application leaders
// in the model.
func (ctx *facadeContext) LeadershipReader() (leadership.Reader, error) {
	reader, err := ctx.r.shared.leaseManager.Reader(
		lease.ApplicationLeadershipNamespace,
		ctx.ModelUUID().String(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return leadershipReader{reader: reader}, nil
}

// HTTPClient returns an HTTP client to use for the given purpose. The following
// errors can be expected:
// - [ErrorHTTPClientPurposeInvalid] when the requested purpose is not
// understood by the context.
// - [ErrorHTTPClientForPurposeNotFound] when no http client can be found for
// the requested [HTTPClientPurpose].
func (ctx *facadeContext) HTTPClient(purpose corehttp.Purpose) (facade.HTTPClient, error) {
	var client facade.HTTPClient

	switch purpose {
	case corehttp.CharmhubPurpose:
		client = ctx.r.shared.charmhubHTTPClient
	default:
		return nil, fmt.Errorf(
			"cannot get http client for purpose %q, purpose is not understood by the facade context%w",
			purpose, errors.Hide(facade.ErrorHTTPClientPurposeInvalid),
		)
	}

	if client == nil {
		return nil, fmt.Errorf(
			"cannot get http client for purpose %q: http client not found%w",
			purpose, errors.Hide(facade.ErrorHTTPClientForPurposeNotFound),
		)
	}

	return client, nil
}

type modelStorageRegistry func(context.Context) (storage.ProviderRegistry, error)

// GetStorageRegistry returns a storage registry for the given namespace.
func (c modelStorageRegistry) GetStorageRegistry(ctx context.Context) (storage.ProviderRegistry, error) {
	return c(ctx)
}

type modelObjectStore func(context.Context) (objectstore.ObjectStore, error)

// GetObjectStore returns the object store for the current model.
func (c modelObjectStore) GetObjectStore(ctx context.Context) (objectstore.ObjectStore, error) {
	return c(ctx)
}

// ModelExporter returns a model exporter for the current model.
func (ctx *facadeContext) ModelExporter(c context.Context, modelUUID model.UUID) (facade.ModelExporter, error) {
	logger := ctx.Logger()
	clock := ctx.r.clock

	domainServices, err := ctx.DomainServicesForModel(c, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	coordinator := coremodelmigration.NewCoordinator(logger)

	objectStoreGetter := modelObjectStore(func(stdCtx context.Context) (objectstore.ObjectStore, error) {
		return ctx.r.objectStoreGetter.GetObjectStore(stdCtx, ctx.ModelUUID().String())
	})

	exporter := modelmigration.NewExporter(
		coordinator,
		modelStorageRegistry(func(ctx context.Context) (storage.ProviderRegistry, error) {
			storageService := domainServices.Storage()
			return storageService.GetStorageRegistry(ctx)
		}),
		objectStoreGetter,
		clock,
		logger,
	)
	return migration.NewModelExporter(
		exporter,
		ctx.migrationScope(modelUUID),
		modelStorageRegistry(func(ctx context.Context) (storage.ProviderRegistry, error) {
			storageService := domainServices.Storage()
			return storageService.GetStorageRegistry(ctx)
		}),
		coordinator,
		logger,
		clock,
	), nil
}

// ModelImporter returns a model importer.
func (ctx *facadeContext) ModelImporter() facade.ModelImporter {
	domainServices := ctx.DomainServices()

	return migration.NewModelImporter(
		ctx.migrationScope,
		ctx.DomainServices().ControllerConfig(),
		ctx.r.domainServicesGetter,
		modelStorageRegistry(func(ctx context.Context) (storage.ProviderRegistry, error) {
			storageService := domainServices.Storage()
			return storageService.GetStorageRegistry(ctx)
		}),
		modelObjectStore(func(stdCtx context.Context) (objectstore.ObjectStore, error) {
			return ctx.r.objectStoreGetter.GetObjectStore(stdCtx, ctx.ModelUUID().String())
		}),
		ctx.Logger(),
		ctx.r.clock,
	)
}

// DomainServices returns the services factory for the current model.
func (ctx *facadeContext) DomainServices() services.DomainServices {
	return ctx.r.domainServices
}

// Tracer returns the tracer for the current model.
func (ctx *facadeContext) Tracer() trace.Tracer {
	return ctx.r.tracer
}

// ObjectStore returns the object store for the current model.
func (ctx *facadeContext) ObjectStore() objectstore.ObjectStore {
	return ctx.r.objectStore
}

// ControllerObjectStore returns the object store for the controller. The
// primary use case for this is agent tools.
func (ctx *facadeContext) ControllerObjectStore() objectstore.ObjectStore {
	return ctx.r.controllerObjectStore
}

// MachineTag returns the current machine tag.
func (ctx *facadeContext) MachineTag() names.Tag {
	return ctx.r.shared.machineTag
}

// DataDir returns the data directory.
func (ctx *facadeContext) DataDir() string {
	return ctx.r.shared.dataDir
}

// LogDir returns the log directory.
func (ctx *facadeContext) LogDir() string {
	return ctx.r.shared.logDir
}

// Logger returns the apiserver logger instance.
func (ctx *facadeContext) Logger() corelogger.Logger {
	return ctx.r.shared.logger
}

// Clock returns the clock instance.
func (ctx *facadeContext) Clock() clock.Clock {
	return ctx.r.clock
}

// controllerDB is a protected method, do not expose this directly in to the
// facade context. It is expect that users of the facade context will use the
// higher level abstractions.
func (ctx *facadeContext) controllerDB(c context.Context) (changestream.WatchableDB, error) {
	db, err := ctx.r.shared.dbGetter.GetWatchableDB(c, coredatabase.ControllerNS)
	return db, errors.Trace(err)
}

// modelDB is a protected method, do not expose this directly in to the
// facade context. It is expected that users of the facade context will use the
// higher level abstractions.
func (ctx *facadeContext) modelDB(c context.Context, modelUUID model.UUID) (changestream.WatchableDB, error) {
	if err := modelUUID.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating model uuid")
	}
	db, err := ctx.r.shared.dbGetter.GetWatchableDB(c, modelUUID.String())
	return db, errors.Trace(err)
}

// migrationScope is a protected method, do not expose this directly in to the
// facade context. It is expect that users of the facade context will use the
// higher level abstractions.
func (ctx *facadeContext) migrationScope(modelUUID model.UUID) coremodelmigration.Scope {
	return coremodelmigration.NewScope(
		changestream.NewTxnRunnerFactory(ctx.controllerDB),
		changestream.NewTxnRunnerFactory(func(c context.Context) (changestream.WatchableDB, error) {
			return ctx.modelDB(c, modelUUID)
		}),
		ctx.r.shared.dbDeleter,
	)
}

// DomainServicesForModel returns the services factory for a given
// model uuid.
func (ctx *facadeContext) DomainServicesForModel(c context.Context, uuid model.UUID) (services.DomainServices, error) {
	return ctx.r.domainServicesGetter.ServicesForModel(c, uuid)
}

// ObjectStoreForModel returns the object store for a given model uuid.
func (ctx *facadeContext) ObjectStoreForModel(stdCtx context.Context, modelUUID string) (objectstore.ObjectStore, error) {
	return ctx.r.objectStoreGetter.GetObjectStore(stdCtx, modelUUID)
}

// DescribeFacades returns the list of available Facades and their Versions
func DescribeFacades(registry *facade.Registry) []params.FacadeVersions {
	facades := registry.List()
	result := make([]params.FacadeVersions, len(facades))
	for i, f := range facades {
		result[i].Name = f.Name
		result[i].Versions = f.Versions
	}
	return result
}
