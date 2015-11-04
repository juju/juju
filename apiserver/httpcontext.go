// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// httpContext provides context for HTTP handlers.
type httpContext struct {
	// strictValidation means that empty envUUID values are not valid.
	strictValidation bool
	// stateServerEnvOnly only validates the state server environment
	stateServerEnvOnly bool
	// srv holds the API server instance.
	srv *Server
}

type errorSender interface {
	sendError(w http.ResponseWriter, code int, err error)
}

var errUnauthorized = errors.NewUnauthorized(nil, "unauthorized")

// stateForRequestUnauthenticated returns a state instance appropriate for
// using for the environment implicit in the given request
// without checking any authentication information.
func (ctxt *httpContext) stateForRequestUnauthenticated(r *http.Request) (*state.State, error) {
	envUUID, err := validateEnvironUUID(validateArgs{
		statePool:          ctxt.srv.statePool,
		envUUID:            r.URL.Query().Get(":envuuid"),
		strict:             ctxt.strictValidation,
		stateServerEnvOnly: ctxt.stateServerEnvOnly,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	st, err := ctxt.srv.statePool.Get(envUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// stateForRequestAuthenticated returns a state instance appropriate for
// using for the environment implicit in the given request.
// It also returns the authenticated entity.
func (ctxt *httpContext) stateForRequestAuthenticated(r *http.Request) (*state.State, state.Entity, error) {
	st, err := ctxt.stateForRequestUnauthenticated(r)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	req, err := ctxt.loginRequest(r)
	if err != nil {
		return nil, nil, errors.NewUnauthorized(err, "")
	}
	entity, _, err := checkCreds(st, req, true, ctxt.srv.authCtxt)
	if err != nil {
		// All errors other than a macaroon-discharge error count as
		// unauthorized at this point.
		if !common.IsDischargeRequiredError(err) {
			err = errors.NewUnauthorized(err, "")
		}
		return nil, nil, errors.Trace(err)
	}
	return st, entity, nil
}

// stateForRequestAuthenticatedUser is like stateForRequestAuthenticated
// except that it also verifies that the authenticated entity is a user.
func (ctxt *httpContext) stateForRequestAuthenticatedUser(r *http.Request) (*state.State, state.Entity, error) {
	st, entity, err := ctxt.stateForRequestAuthenticated(r)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	switch entity.Tag().(type) {
	case names.UserTag:
		return st, entity, nil
	default:
		return nil, nil, errors.Trace(common.ErrBadCreds)
	}
}

// stateForRequestAuthenticatedUser is like stateForRequestAuthenticated
// except that it also verifies that the authenticated entity is a user.
func (ctxt *httpContext) stateForRequestAuthenticatedAgent(r *http.Request) (*state.State, state.Entity, error) {
	st, entity, err := ctxt.stateForRequestAuthenticated(r)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	switch entity.Tag().(type) {
	case names.MachineTag, names.UnitTag:
		return st, entity, nil
	default:
		logger.Errorf("attempt to log in as an agent by %v", entity.Tag())
		return nil, nil, errors.Trace(common.ErrBadCreds)
	}
}

// loginRequest forms a LoginRequest from the information
// in the given HTTP request.
func (ctxt *httpContext) loginRequest(r *http.Request) (params.LoginRequest, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		// No authorization header implies an attempt
		// to login with macaroon authentication.
		return params.LoginRequest{
			Macaroons: httpbakery.RequestMacaroons(r),
		}, nil
	}
	parts := strings.Fields(authHeader)
	if len(parts) != 2 || parts[0] != "Basic" {
		// Invalid header format or no header provided.
		return params.LoginRequest{}, errors.New("invalid request format")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	// See RFC 2617, Section 2.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return params.LoginRequest{}, errors.New("invalid request format")
	}
	tagPass := strings.SplitN(string(challenge), ":", 2)
	if len(tagPass) != 2 {
		return params.LoginRequest{}, errors.New("invalid request format")
	}
	// Ensure that a sensible tag was passed.
	_, err = names.ParseTag(tagPass[0])
	if err != nil {
		return params.LoginRequest{}, errors.Trace(common.ErrBadCreds)
	}
	return params.LoginRequest{
		AuthTag:     tagPass[0],
		Credentials: tagPass[1],
		Nonce:       r.Header.Get(params.MachineNonceHeader),
	}, nil
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
	w.Header().Set("Content-Type", params.ContentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
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
