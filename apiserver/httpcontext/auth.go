// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext

import (
	"context"
	"fmt"
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	internallogger "github.com/juju/juju/internal/logger"
)

// HTTPStrategicAuthenticator is responsible for trying multiple Authenticators
// until one succeeds or an error is returned that is not equal to NotFound.
type HTTPStrategicAuthenticator []authentication.HTTPAuthenticator

// AuthHandler is a http handler responsible for handling authz and authn for
// http requests coming into Juju. If a request both authenticates and authorizes
// then the authentication info is also padded into the http context and the
// next http handler is called.
type AuthHandler struct {
	// NextHandler is the http handler to call after authentication has been
	// completed.
	NextHandler http.Handler

	// Authenticator is the Authenticator used for authenticating
	// the HTTP requests handled by this handler.
	Authenticator authentication.HTTPAuthenticator

	// Authorizer, if non-nil, will be called with the auth info
	// returned by the Authenticator, to validate it for the route.
	Authorizer authentication.Authorizer
}

var logger = internallogger.GetLogger("juju.apiserver.httpcontext")

// ServeHTTP is part of the http.Handler interface and is responsible for
// performing AuthN and AuthZ on the subsequent http request.
func (h *AuthHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	authInfo, err := h.Authenticator.Authenticate(req)
	if err != nil {
		var httpError apiservererrors.HTTPWritableError
		if errors.As(err, &httpError) {
			if err := httpError.SendError(w); err != nil {
				logger.Warningf(context.TODO(), "failed sending http error %v", err)
			}
		} else {
			http.Error(w,
				fmt.Sprintf("authentication failed: %s", err),
				http.StatusUnauthorized,
			)
		}
		return
	}

	if h.Authorizer != nil {
		if err := h.Authorizer.Authorize(req.Context(), authInfo); err != nil {
			http.Error(w,
				fmt.Sprintf("authorization failed: %s", err),
				http.StatusForbidden,
			)
			return
		}
	}

	ctx := context.WithValue(req.Context(), authInfoKey{}, authInfo)
	req = req.WithContext(ctx)
	h.NextHandler.ServeHTTP(w, req)
}

type authInfoKey struct{}

// RequestAuthInfo returns the AuthInfo associated with the context form a
// request. If the context has no auth information associated with it false is
// returned.
func RequestAuthInfo(ctx context.Context) (authentication.AuthInfo, bool) {
	authInfo, ok := ctx.Value(authInfoKey{}).(authentication.AuthInfo)
	return authInfo, ok
}
