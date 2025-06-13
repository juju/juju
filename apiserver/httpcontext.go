// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// httpContext provides context for HTTP handlers.
type httpContext struct {
	// srv holds the API server instance.
	srv *Server
}

// stateForRequestUnauthenticated returns a state instance appropriate for
// using for the model implicit in the given request
// without checking any authentication information.
func (ctxt *httpContext) stateForRequestUnauthenticated(r *http.Request) (*state.PooledState, error) {
	modelUUID := httpcontext.RequestModelUUID(r)
	st, err := ctxt.srv.shared.statePool.Get(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
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
	st, err := ctxt.stateForRequestUnauthenticated(r)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	defer func() {
		// Here err is the named return arg.
		if err != nil {
			st.Release()
		}
	}()
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
	if errors.IsNotFound(err) {
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
	logger.Debugf("sending error: %d %v", statusCode, paramsErr)
	return errors.Trace(sendStatusAndJSON(w, statusCode, &params.ErrorResult{
		Error: paramsErr,
	}))
}

type tagKindAuthorizer []string

// Authorize is part of the httpcontext.Authorizer interface.
func (a tagKindAuthorizer) Authorize(authInfo httpcontext.AuthInfo) error {
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
func (controllerAuthorizer) Authorize(authInfo httpcontext.AuthInfo) error {
	if authInfo.Controller {
		return nil
	}
	return errors.Errorf("%s is not a controller", names.ReadableString(authInfo.Entity.Tag()))
}

type controllerAdminAuthorizer struct {
	st *state.State
}

// Authorize is part of the httpcontext.Authorizer interface.
func (a controllerAdminAuthorizer) Authorize(authInfo httpcontext.AuthInfo) error {
	userTag, ok := authInfo.Entity.Tag().(names.UserTag)
	if !ok {
		return errors.Errorf("%s is not a user", names.ReadableString(authInfo.Entity.Tag()))
	}
	admin, err := a.st.IsControllerAdmin(userTag)
	if err != nil {
		return errors.Trace(err)
	}
	if !admin {
		return errors.Errorf("%s is not a controller admin", names.ReadableString(authInfo.Entity.Tag()))
	}
	return nil
}

// modelPermissionAuthorizer checks that the authenticated user
// has the given permission on a model.
type modelPermissionAuthorizer struct {
	userAccess func(names.UserTag, names.Tag) (permission.Access, error)
	perm       permission.Access
}

// Authorize is part of the httpcontext.Authorizer interface.
func (a modelPermissionAuthorizer) Authorize(authInfo httpcontext.AuthInfo) error {
	userTag, ok := authInfo.Entity.Tag().(names.UserTag)
	if !ok {
		return errors.Errorf("%s is not a user", names.ReadableString(authInfo.Entity.Tag()))
	}
	if !names.IsValidModel(authInfo.ModelTag.Id()) {
		return errors.Errorf("%q is not a valid model", authInfo.ModelTag.Id())
	}
	has, err := common.HasPermission(a.userAccess, userTag, a.perm, authInfo.ModelTag)
	if err != nil {
		return errors.Trace(err)
	}
	if !has {
		return errors.Errorf("%s does not have %q permission", names.ReadableString(authInfo.Entity.Tag()), a.perm)
	}
	return nil
}
