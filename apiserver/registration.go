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
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common"
	coremacaroon "github.com/juju/juju/core/macaroon"
	usererrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
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
			logger.Errorf("%v", err)
		}
		return
	}
	st, err := h.ctxt.stateForRequestUnauthenticated(req)
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	defer st.Release()

	// TODO (stickupkid): Remove this nonsense, we should be able to get the
	// service factory from the handler.
	serviceFactory := h.ctxt.srv.shared.serviceFactoryGetter.FactoryForModel(st.ModelUUID())
	userTag, response, err := h.processPost(
		req,
		st.State,
		serviceFactory.ControllerConfig(),
		serviceFactory.Access(),
		serviceFactory.Cloud(),
		serviceFactory.Credential(),
	)
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}

	// Set a short-lived macaroon as a cookie on the response,
	// which the client can use to obtain a discharge macaroon.
	m, err := h.ctxt.srv.localMacaroonAuthenticator.CreateLocalLoginMacaroon(req.Context(), userTag, httpbakery.RequestVersion(req))
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	cookie, err := httpbakery.NewCookie(coremacaroon.MacaroonNamespace, macaroon.Slice{m})
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	http.SetCookie(w, cookie)

	if err := sendStatusAndJSON(w, http.StatusOK, response); err != nil {
		logger.Errorf("%v", err)
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
	st *state.State,
	controllerConfigService ControllerConfigService,
	userService UserService,
	cloudService common.CloudService, credentialService common.CredentialService,
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
	sealer, err := userService.SetPasswordWithActivationKey(req.Context(), userTag.Name(), loginRequest.Nonce, loginRequest.PayloadCiphertext)
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
	responsePayload, err := h.getSecretKeyLoginResponsePayload(req.Context(), st, controllerConfigService, cloudService, credentialService)
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

func getConnectorInfoer(ctx context.Context, model stateenvirons.Model, cloudService common.CloudService, credentialService common.CredentialService) (environs.ConnectorInfo, error) {
	configGetter := stateenvirons.EnvironConfigGetter{
		Model: model, CloudService: cloudService, CredentialService: credentialService}
	environ, err := common.EnvironFuncForModel(model, cloudService, credentialService, configGetter)(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if connInfo, ok := environ.(environs.ConnectorInfo); ok {
		return connInfo, nil
	}
	return nil, errors.NotSupportedf("environ %q", environ.Config().Type())
}

// For testing.
var GetConnectorInfoer = getConnectorInfoer

// getSecretKeyLoginResponsePayload returns the information required by the
// client to login to the controller securely.
func (h *registerUserHandler) getSecretKeyLoginResponsePayload(
	ctx context.Context,
	st *state.State,
	controllerConfigService ControllerConfigService,
	cloudService common.CloudService,
	credentialService common.CredentialService,
) (*params.SecretKeyLoginResponsePayload, error) {
	if !st.IsController() {
		return nil, errors.New("state is not for a controller")
	}
	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	caCert, _ := controllerConfig.CACert()
	payload := params.SecretKeyLoginResponsePayload{
		CACert:         caCert,
		ControllerUUID: st.ControllerUUID(),
	}

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	connInfo, err := GetConnectorInfoer(ctx, model, cloudService, credentialService)
	if errors.Is(err, errors.NotSupported) { // Not all providers support this.
		return &payload, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	proxier, err := connInfo.ConnectionProxyInfo(ctx)
	if errors.Is(err, errors.NotFound) {
		return &payload, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if payload.ProxyConfig, err = params.NewProxy(proxier); err != nil {
		return nil, errors.Trace(err)
	}
	return &payload, nil
}
