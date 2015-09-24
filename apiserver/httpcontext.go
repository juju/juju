// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// httpContext provides context for HTTP handlers.
type httpContext struct {
	// A cache of State instances for different environments.
	statePool *state.StatePool
	// strictValidation means that empty envUUID values are not valid.
	strictValidation bool
	// stateServerEnvOnly only validates the state server environment
	stateServerEnvOnly bool
}

func (h *httpContext) getEnvironUUID(r *http.Request) string {
	return r.URL.Query().Get(":envuuid")
}

type errorSender interface {
	sendError(w http.ResponseWriter, code int, err error)
}

var errUnauthorized = errors.NewUnauthorized(nil, "unauthorized")

func (h *httpContext) validateEnvironUUID(r *http.Request) (*httpStateWrapper, error) {
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

func authenticatorForTag(tag names.Tag) (authentication.EntityAuthenticator, error) {
	if tag == nil {
		return nil, common.ErrBadRequest
	}
	switch tag.Kind() {
	case names.UserTagKind:
		return &authentication.UserAuthenticator{}, nil
	case names.MachineTagKind, names.UnitTagKind:
		return &authentication.AgentAuthenticator{}, nil
	}
	return nil, common.ErrBadRequest
}

// httpStateWrapper reflects a state connection for a given http connection.
type httpStateWrapper struct {
	state *state.State
}

// authenticate parses HTTP basic authentication and authorizes the
// request by looking up the provided tag and password against state.
func (h *httpStateWrapper) authenticate(r *http.Request) (names.Tag, error) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || parts[0] != "Basic" {
		// Invalid header format or no header provided.
		return nil, errors.New("invalid request format")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	// See RFC 2617, Section 2.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid request format")
	}
	tagPass := strings.SplitN(string(challenge), ":", 2)
	if len(tagPass) != 2 {
		return nil, errors.New("invalid request format")
	}
	// Ensure that a sensible tag was passed.
	tag, err := names.ParseTag(tagPass[0])
	if err != nil {
		return nil, common.ErrBadCreds
	}
	_, _, err = checkCreds(h.state, params.LoginRequest{
		AuthTag:     tagPass[0],
		Credentials: tagPass[1],
		Nonce:       r.Header.Get("X-Juju-Nonce"),
	}, true, authenticatorForTag)
	return tag, err
}

func (h *httpStateWrapper) authenticateUser(r *http.Request) error {
	tag, err := h.authenticate(r)
	if err != nil {
		return err
	}
	switch tag.(type) {
	case names.UserTag:
		return nil
	default:
		return common.ErrBadCreds
	}
}

func (h *httpStateWrapper) authenticateAgent(r *http.Request) (names.Tag, error) {
	tag, err := h.authenticate(r)
	if err != nil {
		return nil, err
	}
	switch tag.(type) {
	case names.MachineTag:
		return tag, nil
	case names.UnitTag:
		return tag, nil
	default:
		return nil, common.ErrBadCreds
	}
}

// sendJSON writes a JSON-encoded response value
// to the given writer along with a trailing newline.
func sendJSON(w io.Writer, response interface{}) {
	body, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("cannot marshal JSON result %#v: %v", response, err)
		return
	}
	body = append(body, '\n')
	w.Write(body)
}

// sendStatusAndJSON sends an HTTP status code and
// a JSON-encoded response to a client.
func sendStatusAndJSON(w http.ResponseWriter, statusCode int, response interface{}) {
	body, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("cannot marshal JSON result %#v: %v", response, err)
		return
	}

	if statusCode == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	}
	w.Header().Set("Content-Type", apihttp.CTypeJSON)
	w.WriteHeader(statusCode)
	w.Write(body)
}

// sendError sends a JSON-encoded error response
// for errors encountered during processing.
func sendError(w http.ResponseWriter, err error) {
	err1, statusCode := common.ServerErrorAndStatus(err)
	logger.Debugf("sending error: %d %v", statusCode, err1)
	sendStatusAndJSON(w, statusCode, &params.ErrorResult{
		Error: err1,
	})
}
