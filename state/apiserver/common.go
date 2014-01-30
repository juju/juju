// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

// errorResponder describes a method returning the wrapped error response instance.
type errorResponder interface {
	// errorResponse wraps the message for an error response.
	errorResponse(message string) interface{}
}

// commonHandler provides common methods for individual handlers.
type commonHandler struct {
	state *state.State
}

// sendJSON sends a JSON-encoded response to the client.
func (h *commonHandler) sendJSON(w http.ResponseWriter, statusCode int, response interface{}) error {
	w.WriteHeader(statusCode)
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	w.Write(body)
	return nil
}

// sendError sends a JSON-encoded error response.
func (h *commonHandler) sendError(responder errorResponder, w http.ResponseWriter, statusCode int, message string, args ...interface{}) error {
	return h.sendJSON(w, statusCode, responder.errorResponse(fmt.Sprintf(message, args...)))
}

// sendAuthError sends an unauthorized error.
func (h *commonHandler) sendAuthError(responder errorResponder, w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	h.sendError(responder, w, http.StatusUnauthorized, "unauthorized")
}

// authenticate provides authentication for the embedding types.
func (h *commonHandler) authenticate(req *http.Request) error {
	return authenticateHTTPRequest(h.state, req)
}

// authenticateHTTPRequest parses HTTP basic authentication and authorizes the
// request by looking up the provided tag and password against the state.
func authenticateHTTPRequest(st *state.State, req *http.Request) error {
	parts := strings.Fields(req.Header.Get("Authorization"))
	if len(parts) != 2 || parts[0] != "Basic" {
		// Invalid header format or no header provided.
		return fmt.Errorf("invalid request format")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	// See RFC 2617, Section 2.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid request format")
	}
	tagPass := strings.SplitN(string(challenge), ":", 2)
	if len(tagPass) != 2 {
		return fmt.Errorf("invalid request format")
	}
	entity, err := checkCreds(st, params.Creds{
		AuthTag:  tagPass[0],
		Password: tagPass[1],
	})
	if err != nil {
		return err
	}
	// Only allow users, not agents.
	_, _, err = names.ParseTag(entity.Tag(), names.UserTagKind)
	if err != nil {
		return common.ErrBadCreds
	}
	return err
}
