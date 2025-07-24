// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"sync"

	"github.com/juju/clock"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
)

var (
	MaxClientPingInterval = maxClientPingInterval
)

func APIHandlerWithEntity(tag names.Tag) *apiHandler {
	return &apiHandler{
		authInfo: authentication.AuthInfo{
			Tag: tag,
		},
	}
}

func NewErrRoot(err error) *errRoot {
	return &errRoot{err}
}

type testingAPIRootHandler struct{}

func (testingAPIRootHandler) State() *state.State {
	return nil
}

func (testingAPIRootHandler) DomainServices() services.DomainServices {
	return nil
}

func (testingAPIRootHandler) DomainServicesGetter() services.DomainServicesGetter {
	return nil
}

func (testingAPIRootHandler) Tracer() coretrace.Tracer {
	return nil
}

func (testingAPIRootHandler) ObjectStore() objectstore.ObjectStore {
	return nil
}

func (testingAPIRootHandler) ObjectStoreGetter() objectstore.ObjectStoreGetter {
	return nil
}

func (testingAPIRootHandler) ControllerObjectStore() objectstore.ObjectStore {
	return nil
}

func (testingAPIRootHandler) SharedContext() *sharedServerContext {
	return nil
}

func (testingAPIRootHandler) Authorizer() facade.Authorizer {
	return nil
}

func (testingAPIRootHandler) ModelUUID() model.UUID {
	return ""
}

// Deprecated: Resources are deprecated. Use WatcherRegistry instead.
func (testingAPIRootHandler) Resources() *common.Resources {
	return common.NewResources()
}

// WatcherRegistry returns a new WatcherRegistry.
func (testingAPIRootHandler) WatcherRegistry() facade.WatcherRegistry {
	return nil
}

func (testingAPIRootHandler) Kill() {}

// TestingAPIRoot gives you an APIRoot as a rpc.Methodfinder that is
// *barely* connected to anything.  Just enough to let you probe some
// of the interfaces, but not enough to actually do any RPC calls.
func TestingAPIRoot(facades *facade.Registry) rpc.Root {
	root, err := newAPIRoot(testingAPIRootHandler{}, facades, nil, clock.WallClock)
	if err != nil {
		// While not ideal, this is only in test code, and there are a bunch of other functions
		// that depend on this one that don't return errors either.
		panic(err)
	}
	return root
}

type StubDomainServicesGetter struct{}

func (s *StubDomainServicesGetter) ServicesForModel(context.Context, model.UUID) (services.DomainServices, error) {
	return nil, nil
}

type StubTracerGetter struct {
	trace.TracerGetter
}

type StubObjectStoreGetter struct {
	objectstore.ObjectStoreGetter
}

// TestingUpgradingRoot returns a resricted srvRoot in an upgrade
// scenario.
func TestingUpgradingRoot() rpc.Root {
	r := TestingAPIRoot(AllFacades())
	return restrictRoot(r, upgradeMethodsOnly)
}

// TestingMigratingRoot returns a resricted srvRoot in a migration
// scenario.
func TestingMigratingRoot() rpc.Root {
	r := TestingAPIRoot(AllFacades())
	return restrictRoot(r, migrationClientMethodsOnly)
}

// TestingAnonymousRoot returns a restricted srvRoot as if
// logged in anonymously.
func TestingAnonymousRoot() rpc.Root {
	r := TestingAPIRoot(AllFacades())
	return restrictRoot(r, anonymousFacadesOnly)
}

// TestingControllerOnlyRoot returns a restricted srvRoot as if
// logged in to the root of the API path.
func TestingControllerOnlyRoot() rpc.Root {
	r := TestingAPIRoot(AllFacades())
	return restrictRoot(r, controllerFacadesOnly)
}

// TestingModelOnlyRoot returns a restricted srvRoot as if
// logged in to a model.
func TestingModelOnlyRoot() rpc.Root {
	r := TestingAPIRoot(AllFacades())
	return restrictRoot(r, modelFacadesOnly)
}

// TestingCAASModelOnlyRoot returns a restricted srvRoot as if
// logged in to a CAAS model.
func TestingCAASModelOnlyRoot() rpc.Root {
	r := TestingAPIRoot(AllFacades())
	return restrictRoot(r, caasModelFacadesOnly)
}

// TestingRestrictedRoot returns a restricted srvRoot.
func TestingRestrictedRoot(check func(string, string) error) rpc.Root {
	r := TestingAPIRoot(AllFacades())
	return restrictRoot(r, check)
}

// ServerWaitGroup exposes the underlying wait group used to track running API calls
// to allow tests to hold a server open.
func ServerWaitGroup(server *Server) *sync.WaitGroup {
	return &server.wg
}

// SetAllowModelAccess updates the server's allowModelAccess attribute.
func SetAllowModelAccess(server *Server, allow bool) {
	server.allowModelAccess = allow
}

// Patcher defines an interface that matches the PatchValue method on
// CleanupSuite
type Patcher interface {
	PatchValue(ptr, value interface{})
}
