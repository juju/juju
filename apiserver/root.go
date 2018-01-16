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

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/juju/state"
)

var (
	// maxClientPingInterval defines the timeframe until the ping timeout
	// closes the monitored connection. TODO(mue): Idea by Roger:
	// Move to API (e.g. params) so that the pinging there may
	// depend on the interval.
	maxClientPingInterval = 3 * time.Minute
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
	entity    state.Entity

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

// newAPIHandler returns a new apiHandler.
func newAPIHandler(srv *Server, st *state.State, rpcConn *rpc.Conn, modelUUID string, connectionID uint64, serverHost string) (*apiHandler, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	r := &apiHandler{
		state:        st,
		model:        m,
		resources:    common.NewResources(),
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
		common.ValueResource{offerAuthCtxt},
	); err != nil {
		return nil, errors.Trace(err)
	}
	return r, nil
}

func (r *apiHandler) getResources() *common.Resources {
	return r.resources
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
	goType    reflect.Type
	creator   func(id string) (reflect.Value, error)
}

// ParamsType defines the parameters that should be supplied to this function.
// See rpcreflect.MethodCaller for more detail.
func (s *srvCaller) ParamsType() reflect.Type {
	return s.objMethod.Params
}

// ReturnType defines the object that is returned from the function.`
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
	state       *state.State
	pool        *state.StatePool
	facades     *facade.Registry
	resources   *common.Resources
	authorizer  facade.Authorizer
	objectMutex sync.RWMutex
	objectCache map[objectKey]reflect.Value
}

// newAPIRoot returns a new apiRoot.
func newAPIRoot(st *state.State, pool *state.StatePool, facades *facade.Registry, resources *common.Resources, authorizer facade.Authorizer) *apiRoot {
	r := &apiRoot{
		state:       st,
		pool:        pool,
		facades:     facades,
		resources:   resources,
		authorizer:  authorizer,
		objectCache: make(map[objectKey]reflect.Value),
	}
	return r
}

// restrictAPIRoot calls restrictAPIRootDuringMaintenance, and
// then restricts the result further to the contoller or model
// facades, depending on the type of login.
func restrictAPIRoot(
	srv *Server,
	apiRoot rpc.Root,
	model *state.Model,
	auth authResult,
) (rpc.Root, error) {
	if !auth.controllerMachineLogin {
		// Controller agents are allowed to
		// connect even during maintenance.
		restrictedRoot, err := restrictAPIRootDuringMaintenance(
			srv, apiRoot, model, auth.tag, auth.controllerMachineLogin,
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
		if model.Type() == state.ModelTypeCAAS {
			apiRoot = restrictRoot(apiRoot, caasModelFacadesOnly)
		}
	}
	return apiRoot, nil
}

// restrictAPIRootDuringMaintenance restricts the API root during
// maintenance events (upgrade, restore, or migration), depending
// on the authenticated client.
func restrictAPIRootDuringMaintenance(
	srv *Server,
	apiRoot rpc.Root,
	model *state.Model,
	authTag names.Tag,
	controllerMachineLogin bool,
) (rpc.Root, error) {
	describeLogin := func() string {
		if authTag == nil {
			return "anonymous login"
		}
		return fmt.Sprintf("login for %s", names.ReadableString(authTag))
	}

	switch status := srv.restoreStatus(); status {
	case state.RestorePending, state.RestoreInProgress:
		if _, ok := authTag.(names.UserTag); ok {
			// Users get access to a limited set of functionality
			// while a restore is pending or in progress.
			if status == state.RestorePending {
				return restrictRoot(apiRoot, aboutToRestoreMethodsOnly), nil
			} else {
				return restrictAll(apiRoot, restoreInProgressError), nil
			}
		}
		// Agent and anonymous logins are blocked during restore.
		return nil, errors.Errorf("%s blocked because restore is in progress", describeLogin())
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
	noMethod := rpcreflect.ObjMethod{}
	goType, err := r.facades.GetType(rootName, version)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, noMethod, &rpcreflect.CallNotImplementedError{
				RootMethod: rootName,
				Version:    version,
			}
		}
		return nil, noMethod, err
	}
	rpcType := rpcreflect.ObjTypeOf(goType)
	objMethod, err := rpcType.Method(methodName)
	if err != nil {
		if err == rpcreflect.ErrMethodNotFound {
			return nil, noMethod, &rpcreflect.CallNotImplementedError{
				RootMethod: rootName,
				Version:    version,
				Method:     methodName,
			}
		}
		return nil, noMethod, err
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

// facadeContext implements facade.Context
type facadeContext struct {
	r   *apiRoot
	key objectKey
}

// Auth is part of of the facade.Context interface.
func (ctx *facadeContext) Auth() facade.Authorizer {
	return ctx.r.authorizer
}

// Dispose is part of of the facade.Context interface.
func (ctx *facadeContext) Dispose() {
	ctx.r.dispose(ctx.key)
}

// Resources is part of of the facade.Context interface.
func (ctx *facadeContext) Resources() facade.Resources {
	return ctx.r.resources
}

// State is part of of the facade.Context interface.
func (ctx *facadeContext) State() *state.State {
	return ctx.r.state
}

// StatePool is part of of the facade.Context interface.
func (ctx *facadeContext) StatePool() *state.StatePool {
	return ctx.r.pool
}

// ID is part of of the facade.Context interface.
func (ctx *facadeContext) ID() string {
	return ctx.key.objId
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
func (r *apiHandler) AuthMachineAgent() bool {
	_, isMachine := r.GetAuthTag().(names.MachineTag)
	return isMachine
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
	return r.entity.Tag() == tag
}

// AuthController returns whether the authenticated user is a
// machine with running the ManageEnviron job.
func (r *apiHandler) AuthController() bool {
	return isMachineWithJob(r.entity, state.JobManageModel)
}

// AuthClient returns whether the authenticated entity is a client
// user.
func (r *apiHandler) AuthClient() bool {
	_, isUser := r.GetAuthTag().(names.UserTag)
	return isUser
}

// GetAuthTag returns the tag of the authenticated entity, if any.
func (r *apiHandler) GetAuthTag() names.Tag {
	if r.entity == nil {
		return nil
	}
	return r.entity.Tag()
}

// ConnectedModel returns the UUID of the model authenticated
// against. It's possible for it to be empty if the login was made
// directly to the root of the API instead of a model endpoint, but
// that method is deprecated.
func (r *apiHandler) ConnectedModel() string {
	return r.modelUUID
}

// HasPermission returns true if the logged in user can perform <operation> on <target>.
func (r *apiHandler) HasPermission(operation permission.Access, target names.Tag) (bool, error) {
	return common.HasPermission(r.state.UserPermission, r.entity.Tag(), operation, target)
}

// UserHasPermission returns true if the passed in user can perform <operation> on <target>.
func (r *apiHandler) UserHasPermission(user names.UserTag, operation permission.Access, target names.Tag) (bool, error) {
	return common.HasPermission(r.state.UserPermission, user, operation, target)
}

// DescribeFacades returns the list of available Facades and their Versions
func DescribeFacades(registry *facade.Registry) []params.FacadeVersions {
	facades := registry.List()
	result := make([]params.FacadeVersions, len(facades))
	for i, facade := range facades {
		result[i].Name = facade.Name
		result[i].Versions = facade.Versions
	}
	return result
}
