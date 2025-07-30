// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	internalhttp "github.com/juju/juju/apiserver/internal/http"
	coreuser "github.com/juju/juju/core/user"
	usererrors "github.com/juju/juju/domain/access/errors"
	proxyerrors "github.com/juju/juju/domain/proxy/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	internalmacaroon "github.com/juju/juju/internal/macaroon"
	"github.com/juju/juju/rpc/params"
)

// registerUserHandler is an http.Handler for the "/register" endpoint. This is
// used to complete a secure user registration process, and provide controller
// login credentials.
type registerUserHandler struct {
	ctxt httpContext
}

// ServeHTTP implements the http.Handler interface.
func (h *registerUserHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		err := sendError(w, errors.MethodNotAllowedf("unsupported method: %q", req.Method))
		if err != nil {
			logger.Errorf(req.Context(), "%v", err)
		}
		return
	}

	// TODO (stickupkid): Remove this nonsense, we should be able to get the
	// domain services from the handler.
	domainServices, err := h.ctxt.srv.shared.domainServicesGetter.ServicesForModel(req.Context(), h.ctxt.srv.shared.controllerModelUUID)
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf(req.Context(), "%v", err)
		}
		return
	}
	userTag, response, err := h.processPost(
		req,
		domainServices.ModelInfo(),
		domainServices.Proxy(),
		domainServices.ControllerConfig(),
		domainServices.Access(),
	)
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf(req.Context(), "%v", err)
		}
		return
	}

	// Set a short-lived macaroon as a cookie on the response,
	// which the client can use to obtain a discharge macaroon.
	m, err := h.ctxt.srv.localMacaroonAuthenticator.CreateLocalLoginMacaroon(req.Context(), userTag, httpbakery.RequestVersion(req))
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf(req.Context(), "%v", err)
		}
		return
	}
	cookie, err := httpbakery.NewCookie(internalmacaroon.MacaroonNamespace, macaroon.Slice{m})
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf(req.Context(), "%v", err)
		}
		return
	}
	http.SetCookie(w, cookie)

	if err := internalhttp.SendStatusAndJSON(w, http.StatusOK, response); err != nil {
		logger.Errorf(req.Context(), "%v", err)
	}
}

// The client will POST to the "/register" endpoint with a JSON-encoded
// params.SecretKeyLoginRequest. This contains the tag of the user they
// are registering, a (supposedly) unique nonce, and a ciphertext which
// is the result of concatenating the user and nonce values, and then
// encrypting and authenticating them with the NaCl Secretbox algorithm.
//
// If the server can decrypt the ciphertext, then it knows the client
// has the required secret key; thus they are authenticated. The client
// does not have the CA certificate for communicating securely with the
// server, and so must also authenticate the server. The server will
// similarly generate a unique nonce and encrypt the response payload
// using the same secret key as the client. If the client can decrypt
// the payload, it knows the server has the required secret key; thus
// it is also authenticated.
//
// NOTE(axw) it is important that the client and server choose their
// own nonces, because reusing a nonce means that the key-stream can
// be revealed.
func (h *registerUserHandler) processPost(
	req *http.Request,
	modelInfoService ModelInfoService,
	proxyService ProxyService,
	controllerConfigService ControllerConfigService,
	userService UserService,
) (
	names.UserTag, *params.SecretKeyLoginResponse, error,
) {
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return names.UserTag{}, nil, errors.Trace(err)
	}
	var loginRequest params.SecretKeyLoginRequest
	if err := json.Unmarshal(data, &loginRequest); err != nil {
		return names.UserTag{}, nil, errors.Trace(err)
	}

	// Basic validation: ensure that the request contains a valid user tag,
	// nonce, and ciphertext of the expected length.
	userTag, err := names.ParseUserTag(loginRequest.User)
	if err != nil {
		return names.UserTag{}, nil, errors.Trace(err)
	}

	// Decrypt the ciphertext with the user's activation key (if it has one).
	sealer, err := userService.SetPasswordWithActivationKey(req.Context(), coreuser.NameFromTag(userTag), loginRequest.Nonce, loginRequest.PayloadCiphertext)
	if err != nil {
		if errors.Is(err, usererrors.ActivationKeyNotValid) {
			return names.UserTag{}, nil, errors.NotValidf("activation key")
		} else if errors.Is(err, usererrors.ActivationKeyNotFound) {
			return names.UserTag{}, nil, errors.NotFoundf("activation key")
		}
		return names.UserTag{}, nil, errors.Trace(err)
	}

	// Respond with the CA-cert and password, encrypted again with the
	// activation key.
	responsePayload, err := h.getSecretKeyLoginResponsePayload(req.Context(), modelInfoService, proxyService, controllerConfigService)
	if err != nil {
		return names.UserTag{}, nil, errors.Trace(err)
	}
	payloadBytes, err := json.Marshal(responsePayload)
	if err != nil {
		return names.UserTag{}, nil, errors.Trace(err)
	}
	if _, err := rand.Read(loginRequest.Nonce); err != nil {
		return names.UserTag{}, nil, errors.Trace(err)
	}

	// Seal the response payload with the user's activation key.
	sealed, err := sealer.Seal(loginRequest.Nonce, payloadBytes)
	if err != nil {
		return names.UserTag{}, nil, errors.Trace(err)
	}
	response := &params.SecretKeyLoginResponse{
		Nonce:             loginRequest.Nonce,
		PayloadCiphertext: sealed,
	}
	return userTag, response, nil
}

// getSecretKeyLoginResponsePayload returns the information required by the
// client to login to the controller securely.
func (h *registerUserHandler) getSecretKeyLoginResponsePayload(
	ctx context.Context,
	modelInfoService ModelInfoService,
	proxyService ProxyService,
	controllerConfigService ControllerConfigService,
) (*params.SecretKeyLoginResponsePayload, error) {
	modelInfo, err := modelInfoService.GetModelInfo(ctx)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	if !modelInfo.IsControllerModel {
		return nil, internalerrors.Capture(errors.New("model is not a controller"))
	}
	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	caCert, _ := controllerConfig.CACert()
	payload := params.SecretKeyLoginResponsePayload{
		CACert:         caCert,
		ControllerUUID: controllerConfig.ControllerUUID(),
	}

	proxier, err := proxyService.GetConnectionProxyInfo(ctx)
	if errors.Is(err, proxyerrors.ProxyInfoNotSupported) ||
		errors.Is(err, proxyerrors.ProxyInfoNotFound) {
		return &payload, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	if payload.ProxyConfig, err = params.NewProxy(proxier); err != nil {
		return nil, errors.Trace(err)
	}
	return &payload, nil
}
