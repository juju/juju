// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"reflect"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
)

type taggedAuthenticator interface {
	state.Entity
	state.Authenticator
}

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

// srvRoot represents a single client's connection to the state
// after it has logged in.
type srvRoot struct {
	state       *state.State
	rpcConn     *rpc.Conn
	resources   *common.Resources
	entity      taggedAuthenticator
	objectCache map[objectKey]reflect.Value
}

// newSrvRoot creates the client's connection representation
// and starts a ping timeout for the monitoring of this
// connection.
func newSrvRoot(root *initialRoot, entity taggedAuthenticator) *srvRoot {
	r := &srvRoot{
		state:       root.srv.state,
		rpcConn:     root.rpcConn,
		resources:   common.NewResources(),
		entity:      entity,
		objectCache: make(map[objectKey]reflect.Value),
	}
	r.resources.RegisterNamed("dataDir", common.StringResource(root.srv.dataDir))
	return r
}

// Kill implements rpc.Killer.  It cleans up any resources that need
// cleaning up to ensure that all outstanding requests return.
func (r *srvRoot) Kill() {
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

/*
// Rsyslog returns an object that provides access to the Rsyslog API
// facade. The id argument is reserved for future use and currently needs to
// be empty.
func (r *srvRoot) Rsyslog(id string) (*rsyslog.RsyslogAPI, error) {
	if id != "" {
		// Safeguard id for possible future use.
		return nil, common.ErrBadId
	}
	return rsyslog.NewRsyslogAPI(r.srv.state, r.resources, r)
}

// Logger returns an object that provides access to the Logger API facade.
// The id argument is reserved for future use and must be empty.
func (r *srvRoot) Logger(id string) (*loggerapi.LoggerAPI, error) {
	if id != "" {
		// TODO: There is no direct test for this
		return nil, common.ErrBadId
	}
	return loggerapi.NewLoggerAPI(r.srv.state, r.resources, r)
}

// Upgrader returns an object that provides access to the Upgrader API facade.
// The id argument is reserved for future use and must be empty.
func (r *srvRoot) Upgrader(id string) (upgrader.Upgrader, error) {
	if id != "" {
		// TODO: There is no direct test for this
		return nil, common.ErrBadId
	}
	// The type of upgrader we return depends on who is asking.
	// Machines get an UpgraderAPI, units get a UnitUpgraderAPI.
	// This is tested in the state/api/upgrader package since there
	// are currently no direct srvRoot tests.
	tag := r.GetAuthTag()
	switch tag.(type) {
	case names.MachineTag:
		return upgrader.NewUpgraderAPI(r.srv.state, r.resources, r)
	case names.UnitTag:
		return upgrader.NewUnitUpgraderAPI(r.srv.state, r.resources, r)
	default:
		// Not a machine or unit.
		return nil, common.ErrPerm
	}
}
*/
// Call takes the object Id and an instance of ParamsType to create an object and place
// a call on its method. It then returns an instance of ResultType.
func (s *srvCaller) Call(objId string, arg reflect.Value) (reflect.Value, error) {
	objVal, err := s.creator(objId)
	if err != nil {
		return reflect.Value{}, err
	}
	return s.objMethod.Call(objVal, arg)
}

// FindMethod looks up the given rootName and version in our facade registry
// and returns a MethodCaller that will be used by the RPC code to place calls on
// that facade.
// FindMethod uses the global registry state/apiserver/common.Facades.
// For more information about how FindMethod should work, see rpc/server.go and
// rpc/rpcreflect/value.go
func (r *srvRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	goType, err := common.Facades.GetType(rootName, version)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, &rpcreflect.CallNotImplementedError{
				RootMethod: rootName,
				Version:    version,
			}
		}
		return nil, err
	}
	rpcType := rpcreflect.ObjTypeOf(goType)
	objMethod, err := rpcType.Method(methodName)
	if err != nil {
		if err == rpcreflect.ErrMethodNotFound {
			return nil, &rpcreflect.CallNotImplementedError{
				RootMethod: rootName,
				Version:    version,
				Method:     methodName,
			}
		}
		return nil, err
	}
	creator := func(id string) (reflect.Value, error) {
		objKey := objectKey{name: rootName, version: version, objId: id}
		if obj, ok := r.objectCache[objKey]; ok {
			return obj, nil
		}
		factory, err := common.Facades.GetFactory(rootName, version)
		if err != nil {
			// We don't check for IsNotFound here, because it
			// should have already been handled in the GetType
			// check.
			return reflect.Value{}, err
		}
		obj, err := factory(r.state, r.resources, r, id)
		if err != nil {
			return reflect.Value{}, err
		}
		objValue := reflect.ValueOf(obj)
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

// AuthMachineAgent returns whether the current client is a machine agent.
func (r *srvRoot) AuthMachineAgent() bool {
	_, ok := r.entity.(*state.Machine)
	return ok
}

// AuthUnitAgent returns whether the current client is a unit agent.
func (r *srvRoot) AuthUnitAgent() bool {
	_, ok := r.entity.(*state.Unit)
	return ok
}

// AuthOwner returns whether the authenticated user's tag matches the
// given entity tag.
func (r *srvRoot) AuthOwner(tag string) bool {
	return r.entity.Tag().String() == tag
}

// AuthEnvironManager returns whether the authenticated user is a
// machine with running the ManageEnviron job.
func (r *srvRoot) AuthEnvironManager() bool {
	return isMachineWithJob(r.entity, state.JobManageEnviron)
}

// AuthClient returns whether the authenticated entity is a client
// user.
func (r *srvRoot) AuthClient() bool {
	_, isUser := r.entity.(*state.User)
	return isUser
}

// GetAuthTag returns the tag of the authenticated entity.
func (r *srvRoot) GetAuthTag() names.Tag {
	return r.entity.Tag()
}

// GetAuthEntity returns the authenticated entity.
func (r *srvRoot) GetAuthEntity() state.Entity {
	return r.entity
}

// DescribeFacades returns the list of available Facades and their Versions
func (r *srvRoot) DescribeFacades() []params.FacadeVersions {
	facades := common.Facades.List()
	result := make([]params.FacadeVersions, len(facades))
	for i, facade := range facades {
		result[i].Name = facade.Name
		result[i].Versions = facade.Versions
	}
	return result
}
