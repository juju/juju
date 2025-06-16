// Copyright 2016-2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery/form"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication"
	internallogger "github.com/juju/juju/internal/logger"
)

type localLoginHandlers struct {
	authCtxt   *authContext
	userTokens map[string]string
}

const (
	formURL = "/form"
)

var (
	logger = internallogger.GetLogger("juju.apiserver.stateauthenticator")
)

// AddHandlers adds the local login handlers to the given mux.
func (h *localLoginHandlers) AddHandlers(mux *apiserverhttp.Mux) {
	dischargeMux := http.NewServeMux()
	discharger := httpbakery.NewDischarger(
		httpbakery.DischargerParams{
			Key:     h.authCtxt.localUserThirdPartyBakeryKey,
			Checker: httpbakery.ThirdPartyCaveatCheckerFunc(h.checkThirdPartyCaveat),
		})
	discharger.AddMuxHandlers(dischargeMux, localUserIdentityLocationPath)

	dischargeMux.Handle(
		localUserIdentityLocationPath+formURL,
		http.HandlerFunc(h.formHandler),
	)
	_ = mux.AddHandler("POST", localUserIdentityLocationPath+"/discharge", dischargeMux)
	_ = mux.AddHandler("GET", localUserIdentityLocationPath+"/publickey", dischargeMux)
	_ = mux.AddHandler("GET", localUserIdentityLocationPath+"/form", dischargeMux)
	_ = mux.AddHandler("POST", localUserIdentityLocationPath+"/form", dischargeMux)
}

func (h *localLoginHandlers) bakeryError(ctx context.Context, w http.ResponseWriter, err error) {
	httpbakery.WriteError(ctx, w, err)
}

func (h *localLoginHandlers) formHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, fmt.Sprintf("%s method not allowed", req.Method), http.StatusMethodNotAllowed)
		return
	}

	ctx := req.Context()
	reqParams := httprequest.Params{
		Response: w,
		Request:  req,
		Context:  ctx,
	}
	loginRequest := form.LoginRequest{}
	if err := httprequest.Unmarshal(reqParams, &loginRequest); err != nil {
		h.bakeryError(ctx, w, errors.Annotate(err, "can't unmarshal login request"))
		return
	}

	username := loginRequest.Body.Form["user"].(string)
	password := loginRequest.Body.Form["password"].(string)
	userTag := names.NewUserTag(username)
	if !userTag.IsLocal() {
		h.bakeryError(ctx, w, errors.NotValidf("non-local username %q", username))
		return
	}

	authenticator := h.authCtxt.authenticator(req.Host)
	if _, _, err := authenticator.Authenticate(ctx, authentication.AuthParams{
		AuthTag:     userTag,
		Credentials: password,
	}); err != nil {
		h.bakeryError(ctx, w, err)
		return
	}

	token, err := newID()
	if err != nil {
		h.bakeryError(ctx, w, errors.Annotate(err, "cannot generate token"))
		return
	}
	h.userTokens[token] = username

	loginResponse := form.LoginResponse{
		Token: &httpbakery.DischargeToken{
			Kind:  "juju_userpass",
			Value: []byte(token),
		},
	}
	_ = httprequest.WriteJSON(w, http.StatusOK, loginResponse)
}

func newID() (string, error) {
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("cannot read random id: %v", err)
	}
	return fmt.Sprintf("%x", id[:]), nil
}

func (h *localLoginHandlers) checkThirdPartyCaveat(stdCtx context.Context, req *http.Request, cavInfo *bakery.ThirdPartyCaveatInfo, token *httpbakery.DischargeToken) ([]checkers.Caveat, error) {
	tag, err := h.authCtxt.CheckLocalLoginCaveat(string(cavInfo.Condition))
	if err != nil {
		return nil, errors.Trace(err)
	}
	if token == nil {
		if err := h.authCtxt.CheckLocalLoginRequest(stdCtx, req); err == nil {
			return h.authCtxt.DischargeCaveats(tag), nil
		}
		err2 := httpbakery.NewInteractionRequiredError(nil, req)
		err2.SetInteraction("juju_userpass", form.InteractionInfo{URL: localUserIdentityLocationPath + formURL})
		return nil, err2
	}

	tokenString := string(token.Value)
	username, ok := h.userTokens[tokenString]
	delete(h.userTokens, tokenString)
	if token.Kind != "juju_userpass" || !ok {
		return nil, errors.Errorf("invalid token %#v", token)
	}

	// Sanity check.
	if tag.Id() != username {
		return nil, errors.Errorf("discharge token for user %q does not match declared user %q", username, tag.Id())
	}
	return h.authCtxt.DischargeCaveats(tag), nil
}
