// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

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
	state *state.State
}

// authenticate parses HTTP basic authentication and authorizes the
// request by looking up the provided tag and password against state.
func (h *httpHandler) authenticate(r *http.Request) error {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || parts[0] != "Basic" {
		// Invalid header format or no header provided.
		return fmt.Errorf("invalid request format")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	// See RFC 2617, Section 2.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid request format")
	}
	tagPass := strings.SplitN(string(challenge), ":", 2)
	if len(tagPass) != 2 {
		return fmt.Errorf("invalid request format")
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

func (h *httpHandler) getEnvironUUID(r *http.Request) string {
	return r.URL.Query().Get(":envuuid")
}

func (h *httpHandler) validateEnvironUUID(r *http.Request) error {
	// Note: this is only true until we have support for multiple
	// environments. For now, there is only one, so we make sure that is
	// the one being addressed.
	envUUID := h.getEnvironUUID(r)
	logger.Tracef("got a request for env %q", envUUID)
	if envUUID == "" {
		return nil
	}
	env, err := h.state.Environment()
	if err != nil {
		logger.Infof("error looking up environment: %v", err)
		return err
	}
	if env.UUID() != envUUID {
		logger.Infof("environment uuid mismatch: %v != %v",
			envUUID, env.UUID())
		return common.UnknownEnvironmentError(envUUID)
	}
	return nil
}

// authError sends an unauthorized error.
func (h *httpHandler) authError(w http.ResponseWriter, sender errorSender) {
	w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	sender.sendError(w, http.StatusUnauthorized, "unauthorized")
}
