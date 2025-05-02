// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/rpc/params"
)

// introspectionHandler is an http.Handler that wraps an http.Handler
// from the worker/introspection package, adding authentication.
type introspectionHandler struct {
	ctx     httpContext
	handler http.Handler
}

// ServeHTTP is part of the http.Handler interface.
func (h introspectionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.checkAuth(r); err != nil {
		if err := sendError(w, err); err != nil {
			logger.Debugf(r.Context(), "%v", err)
		}
		return
	}
	h.handler.ServeHTTP(w, r)
}

func (h introspectionHandler) checkAuth(r *http.Request) error {
	st, entity, err := h.ctx.stateAndEntityForRequestAuthenticatedUser(r)
	if err != nil {
		return err
	}
	defer st.Release()

	// Users with "superuser" access on the controller,
	// or "read" access on the controller model, can
	// access these endpoints.

	svc, err := h.ctx.srv.shared.domainServicesGetter.ServicesForModel(r.Context(), h.ctx.srv.shared.controllerModelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	accessService := svc.Access()

	userPermission := func(ctx context.Context, userName coreuser.Name, target permission.ID) (permission.Access, error) {
		if objectType := target.ObjectType; !(objectType == permission.Controller || objectType == permission.Model) {
			return "", errors.NotValidf("%q as a target", target.ObjectType)
		}

		access, err := accessService.ReadUserAccessLevelForTarget(ctx, userName, target)
		return access, errors.Trace(err)
	}

	ok, err := common.HasPermission(
		r.Context(),
		userPermission,
		entity.Tag(),
		permission.SuperuserAccess,
		st.ControllerTag(),
	)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	ok, err = common.HasPermission(
		r.Context(),
		userPermission,
		entity.Tag(),
		permission.ReadAccess,
		names.NewModelTag(st.ControllerModelUUID()),
	)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}

	return &params.Error{
		Code:    params.CodeForbidden,
		Message: "access denied",
	}
}
