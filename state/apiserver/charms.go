// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

// charmsHandler handles charm upload through HTTPS in the API server.
type charmsHandler struct {
	state *state.State
}

// CharmsResponse is the server response to a charm upload request.
type CharmsResponse struct {
	Code     int    `json:"code,omitempty"`
	Error    string `json:"error,omitempty"`
	CharmURL string `json:"charmUrl,omitempty"`
}

// sendJSON sends a JSON-encoded response to the client.
func (h *charmsHandler) sendJSON(w http.ResponseWriter, response *CharmsResponse) error {
	if response == nil {
		return fmt.Errorf("response is nil")
	}
	w.WriteHeader(response.Code)
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	w.Write(body)
	return nil
}

// sendError sends a JSON-encoded error response.
func (h *charmsHandler) sendError(w http.ResponseWriter, code int, message string) error {
	if code == 0 {
		// Use code 400 by default.
		code = http.StatusBadRequest
	} else if code == http.StatusOK {
		// Dont' report 200 OK.
		code = 0
	}
	err := h.sendJSON(w, &CharmsResponse{Code: code, Error: message})
	if err != nil {
		return err
	}
	return nil
}

// authenticate parses HTTP basic authentication and authorizes the
// request by looking up the provided tag and password against state.
func (h *charmsHandler) authenticate(w http.ResponseWriter, r *http.Request) error {
	if r == nil {
		return fmt.Errorf("invalid request")
	}
	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || parts[0] != "Basic" {
		// Invalid header format or no header provided.
		return fmt.Errorf("invalid request format")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid request format")
	}
	tagPass := strings.SplitN(string(challenge), ":", 2)
	if len(tagPass) != 2 {
		return fmt.Errorf("invalid request format")
	}
	entity, err := checkCreds(h.state, params.Creds{
		AuthTag:  tagPass[0],
		Password: tagPass[1],
	})
	if err != nil {
		return err
	}
	// Only allow users, not agents.
	_, _, err = names.ParseTag(entity.Tag(), names.UserTagKind)
	if err != nil {
		return common.ErrBadCreds
	}
	return err
}

// authError sends an unauthorized error.
func (h *charmsHandler) authError(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	h.sendError(w, http.StatusUnauthorized, "unauthorized")
}

// processPost handles a charm upload POST request after authentication.
func (h *charmsHandler) processPost(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	series := query.Get("series")
	if series == "" {
		h.sendError(w, 0, "expected series= URL argument")
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		h.sendError(w, 0, err.Error())
		return
	}
	// Get the first (and hopefully only) uploaded part to process.
	part, err := reader.NextPart()
	if err == io.EOF {
		h.sendError(w, 0, "expected a single uploaded file, got none")
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("cannot process uploaded file: %v", err), http.StatusBadRequest)
		return
	}
	// Make sure the content type is zip.
	contentType := part.Header.Get("Content-Type")
	if contentType != "application/zip" {
		h.sendError(w, 0, fmt.Sprintf("expected Content-Type: application/zip, got: %v", contentType))
		return
	}
	tempFile, err := ioutil.TempFile("", "charm")
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, fmt.Sprintf("cannot create temp file: %v", err))
		return
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	if _, err := io.Copy(tempFile, part); err != nil {
		h.sendError(w, http.StatusInternalServerError, fmt.Sprintf("error processing file upload: %v", err))
		return
	}
	if _, err := reader.NextPart(); err != io.EOF {
		h.sendError(w, 0, "expected a single uploaded file, got more")
		return
	}
	archive, err := charm.ReadBundle(tempFile.Name())
	if err != nil {
		h.sendError(w, 0, fmt.Sprintf("invalid charm archive: %v", err))
		return
	}
	charmUrl := fmt.Sprintf("local:%s/%s-%d", series, archive.Meta().Name, archive.Revision())
	h.sendJSON(w, &CharmsResponse{Code: http.StatusOK, CharmURL: charmUrl})
}

func (h *charmsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.authenticate(w, r); err != nil {
		h.authError(w)
		return
	}

	switch r.Method {
	case "POST":
		h.processPost(w, r)
	// Possible future extensions, like GET.
	default:
		h.sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", r.Method))
	}
}
