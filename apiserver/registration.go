// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/rand"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names"
	"golang.org/x/crypto/nacl/secretbox"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

const (
	secretboxNonceLength = 24
	secretboxKeyLength   = 32
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
		sendError(w, errors.MethodNotAllowedf("unsupported method: %q", req.Method))
		return
	}
	st, err := h.ctxt.stateForRequestUnauthenticated(req)
	if err != nil {
		sendError(w, err)
		return
	}
	response, err := h.processPost(req, st)
	if err != nil {
		sendError(w, err)
		return
	}
	sendStatusAndJSON(w, http.StatusOK, response)
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
func (h *registerUserHandler) processPost(req *http.Request, st *state.State) (*params.SecretKeyLoginResponse, error) {

	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var loginRequest params.SecretKeyLoginRequest
	if err := json.Unmarshal(data, &loginRequest); err != nil {
		return nil, err
	}

	// Basic validation: ensure that the request contains a valid user tag,
	// nonce, and ciphertext of the expected length.
	userTag, err := names.ParseUserTag(loginRequest.User)
	if err != nil {
		return nil, err
	}
	if len(loginRequest.Nonce) != secretboxNonceLength {
		return nil, errors.NotValidf("nonce")
	}

	// Decrypt the ciphertext with the user's secret key (if it has one).
	user, err := st.User(userTag)
	if err != nil {
		return nil, err
	}
	if len(user.SecretKey()) != secretboxKeyLength {
		return nil, errors.NotFoundf("secret key for user %q", user.Name())
	}
	var key [secretboxKeyLength]byte
	var nonce [secretboxNonceLength]byte
	copy(key[:], user.SecretKey())
	copy(nonce[:], loginRequest.Nonce)
	payloadBytes, ok := secretbox.Open(nil, loginRequest.PayloadCiphertext, &nonce, &key)
	if !ok {
		// Cannot decrypt the ciphertext, which implies that the secret
		// key specified by the client is invalid.
		return nil, errors.NotValidf("secret key")
	}

	// Unmarshal the request payload, which contains the new password to
	// set for the user.
	var requestPayload params.SecretKeyLoginRequestPayload
	if err := json.Unmarshal(payloadBytes, &requestPayload); err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal payload")
	}
	if err := user.SetPassword(requestPayload.Password); err != nil {
		return nil, errors.Annotate(err, "setting new password")
	}

	// Respond with the CA-cert and password, encrypted again with the
	// secret key.
	responsePayload, err := h.getSecretKeyLoginResponsePayload(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	payloadBytes, err = json.Marshal(responsePayload)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, errors.Trace(err)
	}
	response := &params.SecretKeyLoginResponse{
		Nonce:             nonce[:],
		PayloadCiphertext: secretbox.Seal(nil, payloadBytes, &nonce, &key),
	}
	return response, nil
}

// getSecretKeyLoginResponsePayload returns the information required by the
// client to login to the controller securely.
func (h *registerUserHandler) getSecretKeyLoginResponsePayload(
	st *state.State,
) (*params.SecretKeyLoginResponsePayload, error) {
	if !st.IsController() {
		return nil, errors.New("state is not for a controller")
	}
	payload := params.SecretKeyLoginResponsePayload{
		CACert:         st.CACert(),
		ControllerUUID: st.ModelUUID(),
	}
	return &payload, nil
}

// sendError sends a JSON-encoded error response.
func (h *registerUserHandler) sendError(w io.Writer, req *http.Request, err error) {
	if err != nil {
		logger.Errorf("returning error from %s %s: %s", req.Method, req.URL.Path, errors.Details(err))
	}
	sendJSON(w, &params.ErrorResult{Error: common.ServerError(err)})
}
