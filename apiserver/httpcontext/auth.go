// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
)

var logger = loggo.GetLogger("juju.apiserver.httpcontext")

// LocalMacaroonAuthenticator extends Authenticator with a method of
// creating a local login macaroon. The authenticator is expected to
// honour the resulting macaroon.
type LocalMacaroonAuthenticator interface {
	Authenticator

	// CreateLocalLoginMacaroon creates a macaroon that may be
	// provided to a user as proof that they have logged in with
	// a valid username and password. This macaroon may then be
	// used to obtain a discharge macaroon so that the user can
	// log in without presenting their password for a set amount
	// of time.
	CreateLocalLoginMacaroon(context.Context, names.UserTag, bakery.Version) (*macaroon.Macaroon, error)
}

// Authenticator provides an interface for authenticating a request.
//
// TODO(axw) contract should include macaroon discharge error.
//
// If this returns an error, the handler should return StatusUnauthorized.
type Authenticator interface {
	// Authenticate authenticates the given request, returning the
	// auth info.
	//
	// If the request does not contain any authentication details,
	// then an error satisfying errors.IsNotFound will be returned.
	Authenticate(req *http.Request) (AuthInfo, error)

	// AuthenticateLoginRequest authenticates a LoginRequest.
	//
	// TODO(axw) we shouldn't be using params types here.
	AuthenticateLoginRequest(
		ctx context.Context,
		serverHost string,
		modelUUID string,
		req params.LoginRequest,
	) (AuthInfo, error)
}

// Authorizer is a function type for authorizing a request.
//
// If this returns an error, the handler should return StatusForbidden.
type Authorizer interface {
	Authorize(AuthInfo) error
}

// CompositeAuthorizer invokes the underlying authorizers and
// returns success (nil) when the first one succeeds.
// If none are successful, returns [apiservererrors.ErrPerm].
type CompositeAuthorizer []Authorizer

// Authorize is part of the [Authorizer] interface.
func (c CompositeAuthorizer) Authorize(authInfo AuthInfo) error {
	for _, a := range c {
		if err := a.Authorize(authInfo); err == nil {
			return nil
		}
	}
	return apiservererrors.ErrPerm
}

// AuthorizerFunc is a function type implementing Authorizer.
type AuthorizerFunc func(AuthInfo) error

// Authorize is part of the Authorizer interface.
func (f AuthorizerFunc) Authorize(info AuthInfo) error {
	return f(info)
}

// Entity represents a user, machine, or unit that might be
// authenticated.
type Entity interface {
	Tag() names.Tag
}

// AuthInfo is returned by Authenticator and RequestAuthInfo.
type AuthInfo struct {
	// Entity is the user/machine/unit/etc that has authenticated.
	Entity Entity

	// ModelTag is the tag of the model for which access
	// may be required. Not all auth operations will use it,
	// eg checking for controller admin.
	// The model UUID for the tag comes off the login request.
	ModelTag names.ModelTag

	// Controller reports whether or not the authenticated
	// entity is a controller agent.
	Controller bool
}

// BasicAuthHandler is an http.Handler that authenticates requests that
// it handles with a specified Authenticator. The auth details can later
// be retrieved using the top-level RequestAuthInfo function in this package.
type BasicAuthHandler struct {
	http.Handler

	// Authenticator is the Authenticator used for authenticating
	// the HTTP requests handled by this handler.
	Authenticator Authenticator

	// Authorizer, if non-nil, will be called with the auth info
	// returned by the Authenticator, to validate it for the route.
	Authorizer Authorizer
}

// sendStatusAndJSON sends an HTTP status code and
// a JSON-encoded response to a client.
func sendStatusAndJSON(w http.ResponseWriter, statusCode int, response interface{}) error {
	body, err := json.Marshal(response)
	if err != nil {
		return errors.Errorf("cannot marshal JSON result %#v: %v", response, err)
	}

	if statusCode == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	}
	w.Header().Set("Content-Type", params.ContentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(statusCode)
	if _, err := w.Write(body); err != nil {
		return errors.Annotate(err, "cannot write response")
	}
	return nil
}

// sendError sends a JSON-encoded error response
// for errors encountered during processing.
func sendError(w http.ResponseWriter, errToSend error) error {
	paramsErr, statusCode := apiservererrors.ServerErrorAndStatus(errToSend)
	logger.Debugf("sending error: %d %v", statusCode, paramsErr)
	return errors.Trace(sendStatusAndJSON(w, statusCode, &params.ErrorResult{
		Error: paramsErr,
	}))
}

// ServeHTTP is part of the http.Handler interface.
func (h *BasicAuthHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	authInfo, err := h.Authenticator.Authenticate(req)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
		var dischargeError *apiservererrors.DischargeRequiredError
		if errors.As(err, &dischargeError) {
			sendErr := sendError(w, err)
			if sendErr != nil {
				logger.Errorf("%v", sendErr)
			}
			return
		}
		http.Error(w,
			fmt.Sprintf("authentication failed: %s", err),
			http.StatusUnauthorized,
		)
		return
	}
	if h.Authorizer != nil {
		if err := h.Authorizer.Authorize(authInfo); err != nil {
			http.Error(w,
				fmt.Sprintf("authorization failed: %s", err),
				http.StatusForbidden,
			)
			return
		}
	}
	ctx := context.WithValue(req.Context(), authInfoKey{}, authInfo)
	req = req.WithContext(ctx)
	h.Handler.ServeHTTP(w, req)
}

type authInfoKey struct{}

// RequestAuthInfo returns the AuthInfo associated with the request,
// if any, and a boolean indicating whether or not the request was
// authenticated.
func RequestAuthInfo(req *http.Request) (AuthInfo, bool) {
	authInfo, ok := req.Context().Value(authInfoKey{}).(AuthInfo)
	return authInfo, ok
}
