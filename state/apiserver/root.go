// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"reflect"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/juju/state"
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

// srvRoot represents a single client's connection to the state
// after it has logged in.
type srvRoot struct {
	srv       *Server
	rpcConn   *rpc.Conn
	resources *common.Resources
	entity    taggedAuthenticator
}

// newSrvRoot creates the client's connection representation
// and starts a ping timeout for the monitoring of this
// connection.
func newSrvRoot(root *initialRoot, entity taggedAuthenticator) *srvRoot {
	r := &srvRoot{
		srv:       root.srv,
		rpcConn:   root.rpcConn,
		resources: common.NewResources(),
		entity:    entity,
	}
	// Note: Only the Client API is part of srvRoot rather than all
	// the other ones being created on-demand. Why is that?
	r.resources.RegisterNamed("dataDir", common.StringResource(r.srv.dataDir))
	return r
}

// Kill implements rpc.Killer.  It cleans up any resources that need
// cleaning up to ensure that all outstanding requests return.
func (r *srvRoot) Kill() {
	r.resources.StopAll()
}

type srvCaller struct {
	expectedType reflect.Type
	rootName     string
	version      int
	objMethod    rpcreflect.ObjMethod
	state        *state.State
	resources    *common.Resources
	authorizer   common.Authorizer
}

func (s *srvCaller) ParamsType() reflect.Type {
	return s.objMethod.Params
}

func (s *srvCaller) ResultType() reflect.Type {
	return s.objMethod.Result
}

func (s *srvCaller) Call(objId string, arg reflect.Value) (reflect.Value, error) {
	factory, err := common.GetFacadeFactory(s.rootName, s.version)
	if err != nil {
		// Handle errors.IsNotFound again here?
		return reflect.Value{}, err
	}
	obj, err := factory(s.state, s.resources, s.authorizer, objId)
	if err != nil {
		// Handle errors.IsNotFound again here?
		return reflect.Value{}, err
	}
	// TODO: check that the type of obj is exactly what we expected,
	// otherwise objMethod is likely to screw us over (wrong address, wrong
	// offset, etc.)
	logger.Tracef("calling: %T %v %#v", obj, obj, s.objMethod)
	return s.objMethod.Call(reflect.ValueOf(obj), arg)
}

func (r *srvRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	// TODO: We should
	goType, err := common.Facades.GetType(rootName, version)
	if err != nil {
		if errors.IsNotFound(err) {
			// translate this to CallNotImplementedError
		}
		return nil, err
	}
	rpcType := rpcreflect.ObjTypeOf(goType)
	objMethod, err := rpcType.Method(methodName)
	if err != nil {
		return nil, &rpcreflect.CallNotImplementedError{
			RootMethod: rootName,
			Version:    version,
			Method:     methodName,
		}
	}
	return &srvCaller{
		expectedType: goType,
		rootName:     rootName,
		version:      version,
		objMethod:    objMethod,
		state:        r.srv.state,
		resources:    r.resources,
		authorizer:   r,
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
	return r.entity.Tag() == tag
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
func (r *srvRoot) GetAuthTag() string {
	return r.entity.Tag()
}

// GetAuthEntity returns the authenticated entity.
func (r *srvRoot) GetAuthEntity() state.Entity {
	return r.entity
}
