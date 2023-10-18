// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
)

// backupHandler handles backup requests.
type backupHandler struct {
	ctxt httpContext
}

func (h *backupHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	// Validate before authenticate because the authentication is dependent
	// on the state connection that is determined during the validation.
	st, err := h.ctxt.stateForRequestAuthenticatedUser(req)
	if err != nil {
		h.sendError(resp, err)
		return
	}
	defer st.Release()

	if !st.IsController() {
		h.sendError(resp, errors.New("requested model is not the controller model"))
		return
	}

	switch req.Method {
	case "GET":
		logger.Infof("handling backups download request")
		model, err := st.Model()
		if err != nil {
			h.sendError(resp, err)
			return
		}
		modelConfig, err := model.ModelConfig(req.Context())
		if err != nil {
			h.sendError(resp, err)
			return
		}

		// TODO (manadart 2023-10-11): Implement when we have a solution for Dqlite.
		h.sendError(resp, errors.Errorf("not backups in directory %q", modelConfig.BackupDir()))
	default:
		h.sendError(resp, errors.MethodNotAllowedf("unsupported method: %q", req.Method))
	}
}

// sendError sends a JSON-encoded error response.
// Note the difference from the error response sent by
// the sendError function - the error is encoded directly
// rather than in the Error field.
func (h *backupHandler) sendError(w http.ResponseWriter, err error) {
	err, status := apiservererrors.ServerErrorAndStatus(err)
	if err := sendStatusAndJSON(w, status, err); err != nil {
		logger.Errorf("%v", err)
	}
}
