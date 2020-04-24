// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/permission"
	"github.com/juju/names/v4"
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

	ok, err := common.HasPermission(
		st.UserPermission,
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
		st.UserPermission,
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
