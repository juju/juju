// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/permission"
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
			logger.Debugf("%v", err)
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

	accessService := h.ctx.srv.shared.serviceFactoryGetter.FactoryForModel(h.ctx.srv.shared.controllerModelID).Access()

	userPermission := func(subject names.UserTag, target names.Tag) (permission.Access, error) {
		if kind := target.Kind(); !(kind == names.ControllerTagKind || kind == names.ModelTagKind) {
			return "", errors.NotValidf("%q as a target", target.Kind())
		}

		pID, err := permission.ParseTagForID(target)
		if err != nil {
			return "", errors.Trace(err)
		}

		access, err := accessService.ReadUserAccessForTarget(r.Context(), subject.Id(), pID)
		return access.Access, errors.Trace(err)
	}

	ok, err := common.HasPermission(
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
