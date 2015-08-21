// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"reflect"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
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

	// mongoPingInterval defines the interval at which an API server
	// will ping the mongo session to make sure that it's still
	// alive. When the ping returns an error, the server will be
	// terminated.
	mongoPingInterval = 10 * time.Second
)

type objectKey struct {
	name    string
	version int
	objId   string
}

// apiHandler represents a single client's connection to the state
// after it has logged in. It contains an rpc.MethodFinder which it
// uses to dispatch Api calls appropriately.
type apiHandler struct {
	state     *state.State
	rpcConn   *rpc.Conn
	resources *common.Resources
	entity    state.Entity
	// An empty envUUID means that the user has logged in through the
	// root of the API server rather than the /environment/:env-uuid/api
	// path, logins processed with v2 or later will only offer the
	// user manager and environment manager api endpoints from here.
	envUUID string
}

var _ = (*apiHandler)(nil)

// newApiHandler returns a new apiHandler.
func newApiHandler(srv *Server, st *state.State, rpcConn *rpc.Conn, reqNotifier *requestNotifier, envUUID string) (*apiHandler, error) {
	r := &apiHandler{
		state:     st,
		resources: common.NewResources(),
		rpcConn:   rpcConn,
		envUUID:   envUUID,
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
func (s *srvCaller) Call(objId string, arg reflect.Value) (reflect.Value, error) {
	objVal, err := s.creator(objId)
	if err != nil {
		return reflect.Value{}, err
	}
	return s.objMethod.Call(objVal, arg)
}

// apiRoot implements basic method dispatching to the facade registry.
type apiRoot struct {
	state       *state.State
	resources   *common.Resources
	authorizer  common.Authorizer
	objectMutex sync.RWMutex
	objectCache map[objectKey]reflect.Value
}

// newApiRoot returns a new apiRoot.
func newApiRoot(st *state.State, resources *common.Resources, authorizer common.Authorizer) *apiRoot {
	r := &apiRoot{
		state:       st,
		resources:   resources,
		authorizer:  authorizer,
		objectCache: make(map[objectKey]reflect.Value),
	}
	return r
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
		factory, err := common.Facades.GetFactory(rootName, version)
		if err != nil {
			// We don't check for IsNotFound here, because it
			// should have already been handled in the GetType
			// check.
			return reflect.Value{}, err
		}
		obj, err := factory(r.state, r.resources, r.authorizer, id)
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
	goType, err := common.Facades.GetType(rootName, version)
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

// AnonRoot dispatches API calls to those available to an anonymous connection
// which has not logged in.
type anonRoot struct {
	*apiHandler
	adminApis map[int]interface{}
}

// NewAnonRoot creates a new AnonRoot which dispatches to the given Admin API implementation.
func newAnonRoot(h *apiHandler, adminApis map[int]interface{}) *anonRoot {
	r := &anonRoot{
		apiHandler: h,
		adminApis:  adminApis,
	}
	return r
}

func (r *anonRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if rootName != "Admin" {
		return nil, &rpcreflect.CallNotImplementedError{
			RootMethod: rootName,
			Version:    version,
		}
	}
	if api, ok := r.adminApis[version]; ok {
		return rpcreflect.ValueOf(reflect.ValueOf(api)).FindMethod(rootName, 0, methodName)
	}
	return nil, &rpcreflect.CallNotImplementedError{
		RootMethod: rootName,
		Method:     methodName,
		Version:    version,
	}
}

// AuthMachineAgent returns whether the current client is a machine agent.
func (r *apiHandler) AuthMachineAgent() bool {
	_, isMachine := r.GetAuthTag().(names.MachineTag)
	return isMachine
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

// AuthEnvironManager returns whether the authenticated user is a
// machine with running the ManageEnviron job.
func (r *apiHandler) AuthEnvironManager() bool {
	return isMachineWithJob(r.entity, state.JobManageEnviron)
}

// AuthClient returns whether the authenticated entity is a client
// user.
func (r *apiHandler) AuthClient() bool {
	_, isUser := r.GetAuthTag().(names.UserTag)
	return isUser
}

// GetAuthTag returns the tag of the authenticated entity.
func (r *apiHandler) GetAuthTag() names.Tag {
	return r.entity.Tag()
}

// GetAuthEntity returns the authenticated entity.
func (r *apiHandler) GetAuthEntity() state.Entity {
	return r.entity
}

// DescribeFacades returns the list of available Facades and their Versions
func DescribeFacades() []params.FacadeVersions {
	facades := common.Facades.List()
	result := make([]params.FacadeVersions, len(facades))
	for i, facade := range facades {
		result[i].Name = facade.Name
		result[i].Versions = facade.Versions
	}
	return result
}
