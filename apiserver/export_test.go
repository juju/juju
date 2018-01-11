// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
)

var (
	NewPingTimeout        = newPingTimeout
	MaxClientPingInterval = maxClientPingInterval
	NewBackups            = &newBackups
	BZMimeType            = bzMimeType
	JSMimeType            = jsMimeType
	GUIURLPathPrefix      = guiURLPathPrefix
	SpritePath            = spritePath
)

func ServerMacaroon(srv *Server) (*macaroon.Macaroon, error) {
	auth, err := srv.loginAuthCtxt.externalMacaroonAuth()
	if err != nil {
		return nil, err
	}
	return auth.(*authentication.ExternalMacaroonAuthenticator).Macaroon, nil
}

func ServerBakeryService(srv *Server) (authentication.BakeryService, error) {
	auth, err := srv.loginAuthCtxt.externalMacaroonAuth()
	if err != nil {
		return nil, err
	}
	return auth.(*authentication.ExternalMacaroonAuthenticator).Service, nil
}

// ServerAuthenticatorForTag calls the authenticatorForTag method
// of the server's authContext.
func ServerAuthenticatorForTag(srv *Server, tag names.Tag) (authentication.EntityAuthenticator, error) {
	return srv.loginAuthCtxt.authenticator("testing.invalid:1234").authenticatorForTag(tag)
}

func APIHandlerWithEntity(entity state.Entity) *apiHandler {
	return &apiHandler{entity: entity}
}

const (
	LoginRateLimit = defaultLoginRateLimit
	LoginRetyPause = defaultLoginRetryPause
)

// DelayLogins changes how the Login code works so that logins won't proceed
// until they get a message on the returned channel.
// After calling this function, the caller is responsible for sending messages
// on the nextChan in order for Logins to succeed. The original behavior can be
// restored by calling the cleanup function.
func DelayLogins() (nextChan chan struct{}, cleanup func()) {
	nextChan = make(chan struct{}, 10)
	cleanup = func() {
		doCheckCreds = checkCreds
	}
	delayedCheckCreds := func(
		st *state.State,
		c params.LoginRequest,
		authTag names.Tag,
		lookForModelUser bool,
		authenticator authentication.EntityAuthenticator,
	) (state.Entity, *time.Time, error) {
		<-nextChan
		return checkCreds(st, c, authTag, lookForModelUser, authenticator)
	}
	doCheckCreds = delayedCheckCreds
	return
}

func NewErrRoot(err error) *errRoot {
	return &errRoot{err}
}

// TestingAPIRoot gives you an APIRoot as a rpc.Methodfinder that is
// *barely* connected to anything.  Just enough to let you probe some
// of the interfaces, but not enough to actually do any RPC calls.
func TestingAPIRoot(facades *facade.Registry) rpc.Root {
	return newAPIRoot(nil, state.NewStatePool(nil), facades, common.NewResources(), nil)
}

// TestingAPIHandler gives you an APIHandler that isn't connected to
// anything real. It's enough to let test some basic functionality though.
func TestingAPIHandler(c *gc.C, pool *state.StatePool, st *state.State) (*apiHandler, *common.Resources) {
	loginAuthCtxt, err := newAuthContext(pool.SystemState())
	c.Assert(err, jc.ErrorIsNil)
	offerAuthCtxt, err := newOfferAuthcontext(pool)
	c.Assert(err, jc.ErrorIsNil)
	srv := &Server{
		loginAuthCtxt: loginAuthCtxt,
		offerAuthCtxt: offerAuthCtxt,
		statePool:     pool,
		tag:           names.NewMachineTag("0"),
	}
	h, err := newAPIHandler(srv, st, nil, st.ModelUUID(), 6543, "testing.invalid:1234")
	c.Assert(err, jc.ErrorIsNil)
	return h, h.getResources()
}

// TestingAPIHandlerWithEntity gives you the sane kind of APIHandler as
// TestingAPIHandler but sets the passed entity as the apiHandler
// entity.
func TestingAPIHandlerWithEntity(c *gc.C, pool *state.StatePool, st *state.State, entity state.Entity) (*apiHandler, *common.Resources) {
	h, hr := TestingAPIHandler(c, pool, st)
	h.entity = entity
	return h, hr
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

// TestingAboutToRestoreRoot returns a limited root which allows
// methods as per when a restore is about to happen.
func TestingAboutToRestoreRoot() rpc.Root {
	r := TestingAPIRoot(AllFacades())
	return restrictRoot(r, aboutToRestoreMethodsOnly)
}

// Addr returns the address that the server is listening on.
func (srv *Server) Addr() *net.TCPAddr {
	return srv.lis.Addr().(*net.TCPAddr) // cannot fail
}

// PatchGetMigrationBackend overrides the getMigrationBackend function
// to support testing.
func PatchGetMigrationBackend(p Patcher, st migrationBackend) {
	p.PatchValue(&getMigrationBackend, func(*state.State) migrationBackend {
		return st
	})
}

// PatchGetControllerCACert overrides the getControllerCACert function
// to support testing.
func PatchGetControllerCACert(p Patcher, caCert string) {
	p.PatchValue(&getControllerCACert, func(migrationBackend) (string, error) {
		return caCert, nil
	})
}

// Patcher defines an interface that matches the PatchValue method on
// CleanupSuite
type Patcher interface {
	PatchValue(ptr, value interface{})
}

func AssertHasPermission(c *gc.C, handler *apiHandler, access permission.Access, tag names.Tag, expect bool) {
	hasPermission, err := handler.HasPermission(access, tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hasPermission, gc.Equals, expect)
}
