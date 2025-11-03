// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/httpcontext"
	internalhttp "github.com/juju/juju/apiserver/internal/http"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/rpc/params"
)

// httpContext provides context for HTTP handlers.
type httpContext struct {
	// srv holds the API server instance.
	srv *Server
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

// authenticatedUserFromRequest is like authenticatedTagFromRequest
// except that it also verifies that the authenticated entity is a user.
func (ctxt *httpContext) authenticatedUserFromRequest(r *http.Request) (
	names.Tag, error,
) {
	return ctxt.authenticatedTagFromRequest(r, names.UserTagKind)
}

// authenticatedAgentFromRequest is like authenticatedTagFromRequest
// except that it also verifies that the authenticated entity is an agent.
func (ctxt *httpContext) authenticatedAgentFromRequest(r *http.Request) (
	names.Tag, error,
) {
	return ctxt.authenticatedTagFromRequest(r, stateauthenticator.AgentTags...)
}

// authenticatedTagFromRequest checks that the request is correctly authenticated,
// and that the authenticated entity making the request is of one of the
// specified kinds.
func (ctxt *httpContext) authenticatedTagFromRequest(r *http.Request, kinds ...string) (
	names.Tag, error,
) {
	funcs := make([]common.GetAuthFunc, len(kinds))
	for i, kind := range kinds {
		funcs[i] = common.AuthFuncForTagKind(kind)
	}
	authInfo, ok := httpcontext.RequestAuthInfo(r.Context())
	if !ok {
		return nil, apiservererrors.ErrPerm
	}
	authTag := authInfo.Tag
	if ok, err := checkPermissions(r.Context(), authTag, common.AuthAny(funcs...)); !ok {
		return nil, err
	}
	return authTag, nil
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
	return ctxt.srv.catacomb.Dying()
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
