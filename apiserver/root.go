// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/rpcreflect"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/cache"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	jujuversion "github.com/juju/juju/version"
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
	state     *state.State
	model     *state.Model
	rpcConn   *rpc.Conn
	resources *common.Resources
	shared    *sharedServerContext

	// authInfo represents the authentication info established with this client
	// connection.
	authInfo authentication.AuthInfo

	// An empty modelUUID means that the user has logged in through the
	// root of the API server rather than the /model/:model-uuid/api
	// path, logins processed with v2 or later will only offer the
	// user manager and model manager api endpoints from here.
	modelUUID string

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
func newAPIHandler(srv *Server, st *state.State, rpcConn *rpc.Conn, modelUUID string, connectionID uint64, serverHost string) (*apiHandler, error) {
	m, err := st.Model()
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}

		// If this model used to be hosted on this controller but got
		// migrated allow clients to connect and wait for a login
		// request to decide whether the users should be redirected to
		// the new controller for this model or not.
		if _, migErr := st.CompletedMigration(); migErr != nil {
			return nil, errors.Trace(err) // return original NotFound error
		}
	}

	r := &apiHandler{
		state:        st,
		model:        m,
		resources:    common.NewResources(),
		shared:       srv.shared,
		rpcConn:      rpcConn,
		modelUUID:    modelUUID,
		connectionID: connectionID,
		serverHost:   serverHost,
	}

	if err := r.resources.RegisterNamed("machineID", common.StringResource(srv.tag.Id())); err != nil {
		return nil, errors.Trace(err)
	}
	if err := r.resources.RegisterNamed("dataDir", common.StringResource(srv.dataDir)); err != nil {
		return nil, errors.Trace(err)
	}
	if err := r.resources.RegisterNamed("logDir", common.StringResource(srv.logDir)); err != nil {
		return nil, errors.Trace(err)
	}

	// Facades involved with managing application offers need the auth context
	// to mint and validate macaroons.
	localOfferAccessEndpoint := url.URL{
		Scheme: "https",
		Host:   serverHost,
		Path:   localOfferAccessLocationPath,
	}
	offerAuthCtxt := srv.offerAuthCtxt.WithDischargeURL(localOfferAccessEndpoint.String())
	if err := r.resources.RegisterNamed(
		"offerAccessAuthContext",
		common.ValueResource{Value: offerAuthCtxt},
	); err != nil {
		return nil, errors.Trace(err)
	}
	return r, nil
}

// Resources returns the common resources.
func (r *apiHandler) Resources() *common.Resources {
	return r.resources
}

// State returns the underlying state.
func (r *apiHandler) State() *state.State {
	return r.state
}

// SharedContext returns the server shared context.
func (r *apiHandler) SharedContext() *sharedServerContext {
	return r.shared
}

// Authorizer returns the authorizer used for accessing API method calls.
func (r *apiHandler) Authorizer() facade.Authorizer {
	return r
}

func (r *apiHandler) getRpcConn() *rpc.Conn {
	return r.rpcConn
}

// Kill implements rpc.Killer, cleaning up any resources that need
// cleaning up to ensure that all outstanding requests return.
func (r *apiHandler) Kill() {
	r.resources.StopAll()
}

// srvCaller is our implementation of the rpcreflect.MethodCaller interface.
// It lives just long enough to encapsulate the methods that should be
// available for an RPC call and allow the RPC code to instantiate an object
// and place a call on its method.
type srvCaller struct {
	objMethod rpcreflect.ObjMethod
	creator   func(id string) (reflect.Value, error)
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
	objVal, err := s.creator(objId)
	if err != nil {
		return reflect.Value{}, err
	}
	return s.objMethod.Call(ctx, objVal, arg)
}

// apiRoot implements basic method dispatching to the facade registry.
type apiRoot struct {
	clock           clock.Clock
	state           *state.State
	shared          *sharedServerContext
	facades         *facade.Registry
	resources       *common.Resources
	authorizer      facade.Authorizer
	objectMutex     sync.RWMutex
	objectCache     map[objectKey]reflect.Value
	requestRecorder facade.RequestRecorder
}

type apiRootHandler interface {
	// State returns the underlying state.
	State() *state.State
	// SharedContext returns the server shared context.
	SharedContext() *sharedServerContext
	// Resources returns the common resources.
	Resources() *common.Resources
	// Authorizer returns the authorizer used for accessing API method calls.
	Authorizer() facade.Authorizer
}

// newAPIRoot returns a new apiRoot.
func newAPIRoot(clock clock.Clock,
	facades *facade.Registry,
	root apiRootHandler,
	requestRecorder facade.RequestRecorder,
) (*apiRoot, error) {
	st := root.State()
	r := &apiRoot{
		clock:           clock,
		state:           st,
		shared:          root.SharedContext(),
		facades:         facades,
		resources:       root.Resources(),
		authorizer:      root.Authorizer(),
		objectCache:     make(map[objectKey]reflect.Value),
		requestRecorder: requestRecorder,
	}
	// Ensure that the model being requested is in our model cache.
	// Client connections need it for status (or very soon will), and agents
	// require it for model config and others.
	// In all real cases we have a state object, but some test code avoids passing one
	// in, in order to just probe endpoints.
	if st != nil {
		_, err := r.cachedModel(st.ModelUUID())
		if err != nil {
			return nil, errors.Annotate(err, "model cache")
		}
	}
	return r, nil
}

// restrictAPIRoot calls restrictAPIRootDuringMaintenance, and
// then restricts the result further to the controller or model
// facades, depending on the type of login.
func restrictAPIRoot(
	srv *Server,
	apiRoot rpc.Root,
	model *state.Model,
	auth authResult,
	clientVersion version.Number,
) (rpc.Root, error) {
	if !auth.controllerMachineLogin {
		// Controller agents are allowed to
		// connect even during maintenance.
		restrictedRoot, err := restrictAPIRootDuringMaintenance(
			srv, apiRoot, model, auth.tag,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		apiRoot = restrictedRoot
		// If the client version is different to the server version,
		// add extra checks to ensure older incompatible clients cannot be used.
		if clientVersion.Major != jujuversion.Current.Major {
			apiRoot = restrictRoot(apiRoot, checkClientVersion(auth.userLogin, clientVersion))
		}
	}
	if auth.controllerOnlyLogin {
		apiRoot = restrictRoot(apiRoot, controllerFacadesOnly)
	} else {
		apiRoot = restrictRoot(apiRoot, modelFacadesOnly)
		if model.Type() == state.ModelTypeCAAS {
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
	model *state.Model,
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
		switch model.MigrationMode() {
		case state.MigrationModeImporting:
			// The user is not able to access a model that is currently being
			// imported until the model has been activated.
			apiRoot = restrictAll(apiRoot, errors.New("migration in progress, model is importing"))
		case state.MigrationModeExporting:
			// The user is not allowed to change anything in a model that is
			// currently being moved to another controller.
			apiRoot = restrictRoot(apiRoot, migrationClientMethodsOnly)
		}
	}

	return apiRoot, nil
}

// Kill implements rpc.Killer, stopping the root's resources.
func (r *apiRoot) Kill() {
	r.resources.StopAll()
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

	creator := func(id string) (reflect.Value, error) {
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
		obj, err := factory(r.facadeContext(objKey))
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
		if errors.IsNotFound(err) {
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

func (r *apiRoot) cachedModel(uuid string) (*cache.Model, error) {
	model, err := r.shared.controller.WaitForModel(uuid, r.clock)
	if err != nil {
		// Check the database...
		exists, err2 := r.state.ModelExists(uuid)
		if err2 != nil {
			return nil, errors.Trace(err2)
		}
		if exists {
			return nil, errors.Trace(err)
		}
		return nil, errors.NotFoundf("model %q", uuid)
	}
	return model, nil
}

func (r *apiRoot) facadeContext(key objectKey) *facadeContext {
	return &facadeContext{
		r:   r,
		key: key,
	}
}

// facadeContext implements facade.Context
type facadeContext struct {
	r   *apiRoot
	key objectKey
}

// Cancel is part of the facade.Context interface.
func (ctx *facadeContext) Cancel() <-chan struct{} {
	return ctx.r.shared.cancel
}

// Auth is part of the facade.Context interface.
func (ctx *facadeContext) Auth() facade.Authorizer {
	return ctx.r.authorizer
}

// Dispose is part of the facade.Context interface.
func (ctx *facadeContext) Dispose() {
	ctx.r.dispose(ctx.key)
}

// Resources is part of the facade.Context interface.
func (ctx *facadeContext) Resources() facade.Resources {
	return ctx.r.resources
}

// Presence implements facade.Context.
func (ctx *facadeContext) Presence() facade.Presence {
	return ctx
}

// ModelPresence implements facade.ModelPresence.
func (ctx *facadeContext) ModelPresence(modelUUID string) facade.ModelPresence {
	return ctx.r.shared.presence.Connections().ForModel(modelUUID)
}

// Hub implements facade.Context.
func (ctx *facadeContext) Hub() facade.Hub {
	return ctx.r.shared.centralHub
}

// Controller implements facade.Context.
func (ctx *facadeContext) Controller() *cache.Controller {
	return ctx.r.shared.controller
}

// CachedModel implements facade.Context.
func (ctx *facadeContext) CachedModel(uuid string) (*cache.Model, error) {
	return ctx.r.cachedModel(uuid)
}

// State is part of the facade.Context interface.
func (ctx *facadeContext) State() *state.State {
	return ctx.r.state
}

// StatePool is part of the facade.Context interface.
func (ctx *facadeContext) StatePool() *state.StatePool {
	return ctx.r.shared.statePool
}

// MultiwatcherFactory is part of the facade.Context interface.
func (ctx *facadeContext) MultiwatcherFactory() multiwatcher.Factory {
	return ctx.r.shared.multiwatcherFactory
}

// ID is part of the facade.Context interface.
func (ctx *facadeContext) ID() string {
	return ctx.key.objId
}

// RequestRecorder defines a metrics collector for outbound requests.
func (ctx *facadeContext) RequestRecorder() facade.RequestRecorder {
	return ctx.r.requestRecorder
}

// LeadershipClaimer is part of the facade.Context interface.
func (ctx *facadeContext) LeadershipClaimer(modelUUID string) (leadership.Claimer, error) {
	claimer, err := ctx.r.shared.leaseManager.Claimer(
		lease.ApplicationLeadershipNamespace,
		modelUUID,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return leadershipClaimer{claimer}, nil
}

// LeadershipRevoker is part of the facade.Context interface.
func (ctx *facadeContext) LeadershipRevoker(modelUUID string) (leadership.Revoker, error) {
	revoker, err := ctx.r.shared.leaseManager.Revoker(
		lease.ApplicationLeadershipNamespace,
		modelUUID,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return leadershipRevoker{revoker}, nil
}

// LeadershipChecker is part of the facade.Context interface.
func (ctx *facadeContext) LeadershipChecker() (leadership.Checker, error) {
	checker, err := ctx.r.shared.leaseManager.Checker(
		lease.ApplicationLeadershipNamespace,
		ctx.State().ModelUUID(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return leadershipChecker{checker}, nil
}

// LeadershipPinner is part of the facade.Context interface.
// Pinning functionality is only available with the Raft leases implementation.
func (ctx *facadeContext) LeadershipPinner(modelUUID string) (leadership.Pinner, error) {
	pinner, err := ctx.r.shared.leaseManager.Pinner(
		lease.ApplicationLeadershipNamespace,
		modelUUID,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return leadershipPinner{pinner}, nil
}

// LeadershipReader is part of the facade.Context interface.
// It returns a reader that can be used to return all application leaders
// in the model.
func (ctx *facadeContext) LeadershipReader(modelUUID string) (leadership.Reader, error) {
	reader, err := ctx.r.shared.leaseManager.Reader(
		lease.ApplicationLeadershipNamespace,
		modelUUID,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return leadershipReader{reader}, nil
}

// SingularClaimer is part of the facade.Context interface.
func (ctx *facadeContext) SingularClaimer() (lease.Claimer, error) {
	return ctx.r.shared.leaseManager.Claimer(
		lease.SingularControllerNamespace,
		ctx.State().ModelUUID(),
	)
}

func (ctx *facadeContext) HTTPClient(purpose facade.HTTPClientPurpose) facade.HTTPClient {
	switch purpose {
	case facade.CharmhubHTTPClient:
		return ctx.r.shared.charmhubHTTPClient
	default:
		return nil
	}
}

// ControllerDB returns a TrackedDB reference for the controller database.
func (ctx *facadeContext) ControllerDB() (coredatabase.TrackedDB, error) {
	db, err := ctx.r.shared.dbGetter.GetDB(coredatabase.ControllerNS)
	return db, errors.Trace(err)
}

// adminRoot dispatches API calls to those available to an anonymous connection
// which has not logged in, which here is the admin facade.
type adminRoot struct {
	*apiHandler
	adminAPIs map[int]interface{}
}

// newAdminRoot creates a new AnonRoot which dispatches to the given Admin API implementation.
func newAdminRoot(h *apiHandler, adminAPIs map[int]interface{}) *adminRoot {
	r := &adminRoot{
		apiHandler: h,
		adminAPIs:  adminAPIs,
	}
	return r
}

func (r *adminRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if rootName != "Admin" {
		return nil, &rpcreflect.CallNotImplementedError{
			RootMethod: rootName,
			Version:    version,
		}
	}
	if api, ok := r.adminAPIs[version]; ok {
		return rpcreflect.ValueOf(reflect.ValueOf(api)).FindMethod(rootName, 0, methodName)
	}
	return nil, &rpc.RequestError{
		Code:    params.CodeNotSupported,
		Message: "this version of Juju does not support login from old clients",
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
	type hasIsManager interface {
		IsManager() bool
	}
	m, ok := r.authInfo.Entity.(hasIsManager)
	return ok && m.IsManager()
}

// AuthClient returns whether the authenticated entity is a client
// user.
func (r *apiHandler) AuthClient() bool {
	_, isUser := r.GetAuthTag().(names.UserTag)
	return isUser
}

// GetAuthTag returns the tag of the authenticated entity, if any.
func (r *apiHandler) GetAuthTag() names.Tag {
	if r.authInfo.Entity == nil {
		return nil
	}
	return r.authInfo.Entity.Tag()
}

// ConnectedModel returns the UUID of the model authenticated
// against. It's possible for it to be empty if the login was made
// directly to the root of the API instead of a model endpoint, but
// that method is deprecated.
func (r *apiHandler) ConnectedModel() string {
	return r.modelUUID
}

// HasPermission is responsible for reporting if the logged in user is
// able to perform operation x on target y. It uses the authentication mechanism
// of the user to interrogate their permissions. If the entity does not have
// permission to perform the operation then the authentication provider is asked
// to provide a permission error. All permissions errors returned satisfy
// errors.Is(err, ErrorEntityMissingPermission) to distinguish before errors and
// no permissions errors. If error is nil then the user has permission.
func (r *apiHandler) HasPermission(operation permission.Access, target names.Tag) error {
	return r.EntityHasPermission(r.GetAuthTag(), operation, target)
}

// EntityHasPermission is responsible for reporting if the supplied entity is
// able to perform operation x on target y. It uses the authentication mechanism
// of the user to interrogate their permissions. If the entity does not have
// permission to perform the operation then the authentication provider is asked
// to provide a permission error. All permissions errors returned satisfy
// errors.Is(err, ErrorEntityMissingPermission) to distinguish before errors and
// no permissions errors. If error is nil then the user has permission.
func (r *apiHandler) EntityHasPermission(entity names.Tag, operation permission.Access, target names.Tag) error {
	var userAccessFunc common.UserAccessFunc = func(entity names.UserTag, subject names.Tag) (permission.Access, error) {
		if r.authInfo.Delegator == nil {
			return permission.NoAccess, fmt.Errorf("permissions %w for auth info", errors.NotImplemented)
		}
		return r.authInfo.Delegator.SubjectPermissions(authentication.TagToEntity(entity), subject)
	}
	has, err := common.HasPermission(userAccessFunc, entity, operation, target)
	if err != nil && !errors.Is(err, errors.NotFound) {
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
