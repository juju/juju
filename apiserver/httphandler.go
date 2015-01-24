// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// errorSender implementations send errors back to the caller.
type errorSender interface {
	sendError(w http.ResponseWriter, statusCode int, message string)
}

// httpHandler handles http requests through HTTPS in the API server.
type httpHandler struct {
	// rename the state variable to catch all uses of it
	// state server state connection, used for validation
	ssState *state.State
	// strictValidation means that empty envUUID values are not valid.
	strictValidation bool
	// stateServerEnvOnly only validates the state server environment
	stateServerEnvOnly bool
}

// httpStateWrapper reflects a state connection for a given http connection.
type httpStateWrapper struct {
	state       *state.State
	cleanupFunc func()
}

func (h *httpHandler) getEnvironUUID(r *http.Request) string {
	return r.URL.Query().Get(":envuuid")
}

// authError sends an unauthorized error.
func (h *httpHandler) authError(w http.ResponseWriter, sender errorSender) {
	w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	sender.sendError(w, http.StatusUnauthorized, "unauthorized")
}

func (h *httpHandler) validateEnvironUUID(r *http.Request) (*httpStateWrapper, error) {
	envUUID := h.getEnvironUUID(r)
	envState, needsClosing, err := validateEnvironUUID(validateArgs{
		st:                 h.ssState,
		envUUID:            envUUID,
		strict:             h.strictValidation,
		stateServerEnvOnly: h.stateServerEnvOnly,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	wrapper := &httpStateWrapper{state: envState}
	if needsClosing {
		wrapper.cleanupFunc = func() {
			logger.Debugf("close connection to environment: %s", envState.EnvironUUID())
			envState.Close()
		}
	}
	return wrapper, nil
}

// authenticate parses HTTP basic authentication and authorizes the
// request by looking up the provided tag and password against state.
func (h *httpStateWrapper) authenticate(r *http.Request) error {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || parts[0] != "Basic" {
		// Invalid header format or no header provided.
		return errors.New("invalid request format")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	// See RFC 2617, Section 2.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return errors.New("invalid request format")
	}
	tagPass := strings.SplitN(string(challenge), ":", 2)
	if len(tagPass) != 2 {
		return errors.New("invalid request format")
	}
	// Only allow users, not agents.
	if _, err := names.ParseUserTag(tagPass[0]); err != nil {
		return common.ErrBadCreds
	}
	// Ensure the credentials are correct.
	_, err = checkCreds(h.state, params.LoginRequest{
		AuthTag:     tagPass[0],
		Credentials: tagPass[1],
	})
	return err
}

func (h *httpStateWrapper) cleanup() {
	if h.cleanupFunc != nil {
		h.cleanupFunc()
	}
}
