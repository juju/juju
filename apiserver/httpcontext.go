// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/httpcontext"
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

// objectStoreForRequest returns an object store instance appropriate
// for using for the model implicit in the given context supplied from
// a request without checking any authentication information.
func (ctxt *httpContext) objectStoreForRequest(ctx context.Context) (objectstore.ObjectStore, error) {
	modelUUID, valid := httpcontext.RequestModelUUID(ctx)
	if !valid {
		return nil, errors.Trace(apiservererrors.ErrPerm)
	}
	return ctxt.srv.shared.objectStoreGetter.GetObjectStore(ctx, modelUUID)
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
	return ctxt.srv.shared.domainServicesGetter.ServicesForModel(model.UUID(modelUUID)), nil
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
	authInfo, ok := httpcontext.RequestAuthInfo(r)
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
func checkPermissions(tag names.Tag, acceptFunc common.GetAuthFunc) (bool, error) {
	accept, err := acceptFunc()
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
	modelUUID := r.Header.Get(params.MigrationModelHTTPHeader)
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
	model, err := migrationSt.Model()
	if errors.Is(err, errors.NotFound) {
		return nil, fmt.Errorf("%w: %q", apiservererrors.UnknownModelError, modelUUID)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if model.MigrationMode() != requiredMode {
		return nil, errors.BadRequestf(
			"model migration mode is %q instead of %q", model.MigrationMode(), requiredMode)
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
	if ok, err := checkPermissions(entity.Tag(), common.AuthAny(funcs...)); !ok {
		st.Release()
		return nil, nil, err
	}
	return st, entity, nil
}

// stop returns a channel which will be closed when a handler should
// exit.
func (ctxt *httpContext) stop() <-chan struct{} {
	return ctxt.srv.tomb.Dying()
}

// sendStatusAndHeadersAndJSON send an HTTP status code, custom headers
// and a JSON-encoded response to a client
func sendStatusAndHeadersAndJSON(w http.ResponseWriter, statusCode int, headers map[string]string, response interface{}) error {
	for k, v := range headers {
		if !strings.HasPrefix(k, "Juju-") {
			return errors.Errorf(`Custom header %q must be prefixed with "Juju-"`, k)
		}
		w.Header().Set(k, v)
	}
	return sendStatusAndJSON(w, statusCode, response)
}

// sendStatusAndJSON sends an HTTP status code and
// a JSON-encoded response to a client.
func sendStatusAndJSON(w http.ResponseWriter, statusCode int, response interface{}) error {
	body, err := json.Marshal(response)
	if err != nil {
		return errors.Errorf("cannot marshal JSON result %#v: %v", response, err)
	}

	if statusCode == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	}
	w.Header().Set("Content-Type", params.ContentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(statusCode)
	if _, err := w.Write(body); err != nil {
		return errors.Annotate(err, "cannot write response")
	}
	return nil
}

// sendError sends a JSON-encoded error response
// for errors encountered during processing.
func sendError(w http.ResponseWriter, errToSend error) error {
	paramsErr, statusCode := apiservererrors.ServerErrorAndStatus(errToSend)
	logger.Debugf(context.TODO(), "sending error: %d %v", statusCode, paramsErr)
	return errors.Trace(sendStatusAndJSON(w, statusCode, &params.ErrorResult{
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
