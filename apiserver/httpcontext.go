// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/httpcontext"
	internalhttp "github.com/juju/juju/apiserver/internal/http"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// httpContext provides context for HTTP handlers.
type httpContext struct {
	// srv holds the API server instance.
	srv *Server
}

// stateForRequestUnauthenticated returns a state instance appropriate for
// using for the model implicit in the given context supplied from a request
// without checking any authentication information.
func (ctxt *httpContext) stateForRequestUnauthenticated(ctx context.Context) (*state.PooledState, error) {
	modelUUID, valid := httpcontext.RequestModelUUID(ctx)
	if !valid {
		return nil, errors.Trace(apiservererrors.ErrPerm)
	}
	st, err := ctxt.statePool().Get(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// controllerObjectStoreForRequest returns an object store instance
// appropriate for using for the controller model without checking
// any authentication information.
func (ctxt *httpContext) controllerObjectStoreForRequest(ctx context.Context) (objectstore.ObjectStore, error) {
	st, err := ctxt.statePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ctxt.srv.shared.objectStoreGetter.GetObjectStore(ctx, st.ControllerModelUUID())
}

// domainServicesForRequest returns a domain services appropriate for using
// for the model implicit in the given context supplied from a request without
// checking any authentication information.
func (ctxt *httpContext) domainServicesForRequest(ctx context.Context) (services.DomainServices, error) {
	modelUUID, valid := httpcontext.RequestModelUUID(ctx)
	if !valid {
		return nil, errors.Trace(apiservererrors.ErrPerm)
	}
	return ctxt.srv.shared.domainServicesGetter.ServicesForModel(ctx, model.UUID(modelUUID))
}

// objectStoreForRequest returns an object store instance
// appropriate for using for the controller model without checking
// any authentication information.
func (ctxt *httpContext) objectStoreForRequest(ctx context.Context) (objectstore.ObjectStore, error) {
	modelUUID, valid := httpcontext.RequestModelUUID(ctx)
	if !valid {
		return nil, errors.Trace(apiservererrors.ErrPerm)
	}
	return ctxt.srv.shared.objectStoreGetter.GetObjectStore(ctx, modelUUID)
}

// statePool returns the StatePool for this controller.
func (ctxt *httpContext) statePool() *state.StatePool {
	return ctxt.srv.shared.statePool
}

// stateForRequestAuthenticated returns a state instance appropriate for
// using for the model implicit in the given request.
// It also returns the authenticated entity.
func (ctxt *httpContext) stateForRequestAuthenticated(r *http.Request) (
	resultSt *state.PooledState,
	resultEntity state.Entity,
	err error,
) {
	authInfo, ok := httpcontext.RequestAuthInfo(r.Context())
	if !ok {
		return nil, nil, apiservererrors.ErrPerm
	}
	st, err := ctxt.stateForRequestUnauthenticated(r.Context())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return st, authInfo.Entity, nil
}

// checkPermissions verifies that given tag passes authentication check.
// For example, if only user tags are accepted, all other tags will be denied access.
func checkPermissions(ctx context.Context, tag names.Tag, acceptFunc common.GetAuthFunc) (bool, error) {
	accept, err := acceptFunc(ctx)
	if err != nil {
		return false, errors.Trace(err)
	}
	if accept(tag) {
		return true, nil
	}
	return false, errors.NotValidf("tag kind %v", tag.Kind())
}

// stateForMigration asserts that the incoming connection is from a user that
// has admin permissions on the controller model. The method also gets the
// model uuid for the model being migrated from a request header, and returns
// the state instance for that model.
func (ctxt *httpContext) stateForMigration(
	r *http.Request,
	requiredMode state.MigrationMode,
) (_ *state.PooledState, err error) {
	modelUUID, _ := httpcontext.MigrationRequestModelUUID(r)
	migrationSt, err := ctxt.srv.shared.statePool.Get(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		// Here err is the named return arg.
		if err != nil {
			migrationSt.Release()
		}
	}()
	mode, err := migrationSt.MigrationMode()
	if errors.Is(err, errors.NotFound) {
		return nil, fmt.Errorf("%w: %q", apiservererrors.UnknownModelError, modelUUID)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if mode != requiredMode {
		return nil, errors.BadRequestf(
			"model migration mode is %q instead of %q", mode, requiredMode)
	}
	return migrationSt, nil
}

func (ctxt *httpContext) stateForMigrationImporting(r *http.Request) (*state.PooledState, error) {
	return ctxt.stateForMigration(r, state.MigrationModeImporting)
}

// stateForRequestAuthenticatedUser is like stateAndEntityForRequestAuthenticatedUser
// but doesn't return the entity.
func (ctxt *httpContext) stateForRequestAuthenticatedUser(r *http.Request) (*state.PooledState, error) {
	st, _, err := ctxt.stateAndEntityForRequestAuthenticatedUser(r)
	return st, err
}

// stateAndEntityForRequestAuthenticatedUser is like stateForRequestAuthenticated
// except that it also verifies that the authenticated entity is a user.
func (ctxt *httpContext) stateAndEntityForRequestAuthenticatedUser(r *http.Request) (
	*state.PooledState, state.Entity, error,
) {
	return ctxt.stateForRequestAuthenticatedTag(r, names.UserTagKind)
}

// stateForRequestAuthenticatedAgent is like stateForRequestAuthenticated
// except that it also verifies that the authenticated entity is an agent.
func (ctxt *httpContext) stateForRequestAuthenticatedAgent(r *http.Request) (
	*state.PooledState, state.Entity, error,
) {
	return ctxt.stateForRequestAuthenticatedTag(r, stateauthenticator.AgentTags...)
}

// stateForRequestAuthenticatedTag checks that the request is
// correctly authenticated, and that the authenticated entity making
// the request is of one of the specified kinds.
func (ctxt *httpContext) stateForRequestAuthenticatedTag(r *http.Request, kinds ...string) (
	*state.PooledState, state.Entity, error,
) {
	funcs := make([]common.GetAuthFunc, len(kinds))
	for i, kind := range kinds {
		funcs[i] = common.AuthFuncForTagKind(kind)
	}
	st, entity, err := ctxt.stateForRequestAuthenticated(r)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if ok, err := checkPermissions(r.Context(), entity.Tag(), common.AuthAny(funcs...)); !ok {
		st.Release()
		return nil, nil, err
	}
	return st, entity, nil
}

// domainServicesDuringMigrationForRequest returns the domain services for the
// model being migrated, as indicated by the request header.
func (ctxt *httpContext) domainServicesDuringMigrationForRequest(r *http.Request) (services.DomainServices, error) {
	modelUUID, found := httpcontext.MigrationRequestModelUUID(r)
	if !found {
		return nil, errors.Trace(apiservererrors.ErrPerm)
	}
	return ctxt.srv.shared.domainServicesGetter.ServicesForModel(r.Context(), model.UUID(modelUUID))
}

// stop returns a channel which will be closed when a handler should
// exit.
func (ctxt *httpContext) stop() <-chan struct{} {
	return ctxt.srv.tomb.Dying()
}

// sendError sends a JSON-encoded error response
// for errors encountered during processing.
func sendError(w http.ResponseWriter, errToSend error) error {
	paramsErr, statusCode := apiservererrors.ServerErrorAndStatus(errToSend)
	logger.Debugf(context.TODO(), "sending error: %d %v", statusCode, paramsErr)
	return errors.Trace(internalhttp.SendStatusAndJSON(w, statusCode, &params.ErrorResult{
		Error: paramsErr,
	}))
}

type tagKindAuthorizer []string

// Authorize is part of the httpcontext.Authorizer interface.
func (a tagKindAuthorizer) Authorize(_ context.Context, authInfo authentication.AuthInfo) error {
	tagKind := authInfo.Entity.Tag().Kind()
	for _, kind := range a {
		if tagKind == kind {
			return nil
		}
	}
	return errors.NotValidf("tag kind %v", tagKind)
}

type controllerAuthorizer struct{}

// Authorize is part of the httpcontext.Authorizer interface.
func (controllerAuthorizer) Authorize(_ context.Context, authInfo authentication.AuthInfo) error {
	if authInfo.Controller {
		return nil
	}
	return errors.Errorf("%s is not a controller", names.ReadableString(authInfo.Entity.Tag()))
}

type controllerAdminAuthorizer struct {
	controllerTag names.Tag
}

// Authorize is part of the httpcontext.Authorizer interface.
func (a controllerAdminAuthorizer) Authorize(ctx context.Context, authInfo authentication.AuthInfo) error {
	userTag, ok := authInfo.Entity.Tag().(names.UserTag)
	if !ok {
		return errors.Errorf("%s is not a user", names.ReadableString(authInfo.Entity.Tag()))
	}

	has, err := common.HasPermission(ctx,
		func(ctx context.Context, userName user.Name, subject permission.ID) (permission.Access, error) {
			if userName.Name() != userTag.Id() {
				return permission.NoAccess, fmt.Errorf("expected user %q got %q", userTag.String(), userName)
			}
			return authInfo.SubjectPermissions(ctx, subject)
		},
		userTag, permission.SuperuserAccess, a.controllerTag,
	)
	if err != nil {
		return errors.Trace(err)
	}
	if !has {
		return errors.Errorf("%s is not a controller admin", names.ReadableString(authInfo.Entity.Tag()))
	}
	return nil
}
