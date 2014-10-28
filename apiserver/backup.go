// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/juju/errors"

	apibackups "github.com/juju/juju/apiserver/backups"
	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
)

// TODO(ericsnow) This code should be in the backups package.

var newBackups = func(st *state.State) (backups.Backups, error) {
	return apibackups.NewBackups(st)
}

// backupHandler handles backup requests.
type backupHandler struct {
	httpHandler
}

func (h *backupHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if err := h.validateEnvironUUID(req); err != nil {
		h.sendError(resp, http.StatusNotFound, err.Error())
		return
	}

	if err := h.authenticate(req); err != nil {
		h.authError(resp, h)
		return
	}

	backups, err := newBackups(h.state)
	if err != nil {
		h.sendError(resp, http.StatusInternalServerError, err.Error())
		return
	}

	switch req.Method {
	case "GET":
		args, err := h.parseGETArgs(req)
		if err != nil {
			h.sendError(resp, http.StatusInternalServerError, err.Error())
			return
		}

		meta, archive, err := backups.Get(args.ID)
		if err != nil {
			h.sendError(resp, http.StatusInternalServerError, err.Error())
			return
		}
		defer archive.Close()

		err = h.sendFile(archive, meta.Checksum(), apihttp.DIGEST_SHA, resp)
		if err != nil {
			h.sendError(resp, http.StatusInternalServerError, err.Error())
			return
		}
	default:
		h.sendError(resp, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", req.Method))
	}
}

func (h *backupHandler) read(req *http.Request, expectedType string) ([]byte, error) {
	defer req.Body.Close()

	ctype := req.Header.Get("Content-Type")
	if ctype != expectedType {
		return nil, errors.Errorf("expected Content-Type %q, got %q", expectedType, ctype)
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, errors.Annotate(err, "while reading request body")
	}

	return body, nil
}

func (h *backupHandler) parseGETArgs(req *http.Request) (*params.BackupsDownloadArgs, error) {
	body, err := h.read(req, apihttp.CTYPE_JSON)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var args params.BackupsDownloadArgs
	if err := json.Unmarshal(body, &args); err != nil {
		return nil, errors.Annotate(err, "while de-serializing args")
	}

	return &args, nil
}

func (h *backupHandler) sendFile(file io.Reader, checksum string, algorithm apihttp.DigestAlgorithm, resp http.ResponseWriter) error {
	// We don't set the Content-Length header, leaving it at -1.
	resp.Header().Set("Content-Type", apihttp.CTYPE_RAW)
	resp.Header().Set("Digest", fmt.Sprintf("%s=%s", algorithm, checksum))
	resp.WriteHeader(http.StatusOK)
	if _, err := io.Copy(resp, file); err != nil {
		return errors.Annotate(err, "while streaming archive")
	}
	return nil
}

// sendError sends a JSON-encoded error response.
func (h *backupHandler) sendError(w http.ResponseWriter, statusCode int, message string) {
	failure := params.Error{
		Message: message,
		// Leave Code empty.
	}

	body, err := json.Marshal(&failure)
	if err != nil {
		logger.Errorf("failed to serialize the failure (%v): %v", failure, err)
		return
	}

	w.Header().Set("Content-Type", apihttp.CTYPE_JSON)
	w.WriteHeader(statusCode)
	w.Write(body)
}
