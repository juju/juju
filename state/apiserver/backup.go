// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	"net/http"

	"github.com/juju/juju/state/api/params"
)

// charmsHandler handles charm upload through HTTPS in the API server.
type backupHandler struct {
	httpHandler
}

func (h *backupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.authenticate(r); err != nil {
		h.authError(w, h)
		return
	}

	switch r.Method {
	case "POST":
		h.sendJSON(w, http.StatusOK, &params.ToolsResult{
			Tools: agentTools,
			DisableSSLHostnameVerification: disableSSLHostnameVerification,
		})
	default:
		h.sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", r.Method))
	}
}
