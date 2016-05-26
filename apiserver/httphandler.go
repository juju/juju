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
	// A cache of State instances for different environments.
	statePool *state.StatePool
	// strictValidation means that empty envUUID values are not valid.
	strictValidation bool
	// stateServerEnvOnly only validates the state server environment
	stateServerEnvOnly bool
}

// httpStateWrapper reflects a state connection for a given http connection.
type httpStateWrapper struct {
	state *state.State
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
	envState, err := validateEnvironUUID(validateArgs{
		statePool:          h.statePool,
		envUUID:            envUUID,
		strict:             h.strictValidation,
		stateServerEnvOnly: h.stateServerEnvOnly,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &httpStateWrapper{state: envState}, nil
}

// authenticate parses HTTP basic authentication and authorizes the
// request by looking up the provided tag and password against state.
func (h *httpStateWrapper) authenticate(r *http.Request, auth authorisation) (names.Tag, error) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || parts[0] != "Basic" {
		// Invalid header format or no header provided.
		return nil, errors.NotValidf("request format")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	// See RFC 2617, Section 2.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.NotValidf("request format")
	}
	tagPass := strings.SplitN(string(challenge), ":", 2)
	if len(tagPass) != 2 {
		return nil, errors.NotValidf("request format")
	}
	// Ensure that a sensible tag was passed.
	tag, err := names.ParseTag(tagPass[0])
	if err != nil {
		return nil, err
	}
	if ok, err := auth.checkPermissions(tag); !ok {
		return nil, err
	}
	_, _, err = checkCreds(h.state, params.LoginRequest{
		AuthTag:     tagPass[0],
		Credentials: tagPass[1],
		Nonce:       r.Header.Get("X-Juju-Nonce"),
	}, true)
	return tag, err
}

func (h *httpStateWrapper) authenticateUser(r *http.Request) error {
	allow := authorisation{common.AuthFuncForTagKind(names.UserTagKind), "not accepted"}
	if _, err := h.authenticate(r, allow); err != nil {
		return err
	}
	return nil
}

func (h *httpStateWrapper) authenticateAgent(r *http.Request) (names.Tag, error) {
	authFunc := common.AuthEither(
		common.AuthFuncForTagKind(names.MachineTagKind),
		common.AuthFuncForTagKind(names.UnitTagKind),
	)

	tag, err := h.authenticate(r, authorisation{authFunc, "not accepted"})
	if err != nil {
		return nil, err
	}
	return tag, nil
}

type authorisation struct {
	acceptFunc    common.AuthFunc
	deniedMessage string
}

func (a *authorisation) checkPermissions(tag names.Tag) (bool, error) {
	accept, err := a.acceptFunc()
	if err != nil {
		return false, errors.Trace(err)
	}
	if accept(tag) {
		return true, nil
	}
	return false, errors.Errorf("tag %v %v", tag, a.deniedMessage)
}
