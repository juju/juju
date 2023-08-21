// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/backups"
)

var newBackups = backups.NewBackups

// backupHandler handles backup requests.
type backupHandler struct {
	ctxt httpContext
}

func (h *backupHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	// Validate before authenticate because the authentication is dependent
	// on the state connection that is determined during the validation.
	st, err := h.ctxt.stateForRequestAuthenticatedUser(req)
	if err != nil {
		h.sendError(resp, err)
		return
	}
	defer st.Release()

	if !st.IsController() {
		h.sendError(resp, errors.New("requested model is not the controller model"))
		return
	}

	switch req.Method {
	case "GET":
		logger.Infof("handling backups download request")
		model, err := st.Model()
		if err != nil {
			h.sendError(resp, err)
			return
		}
		modelConfig, err := model.ModelConfig(req.Context())
		if err != nil {
			h.sendError(resp, err)
			return
		}
		backupDir := backups.BackupDirToUse(modelConfig.BackupDir())
		paths := &corebackups.Paths{
			BackupDir: backupDir,
		}
		id, err := h.download(newBackups(paths), resp, req)
		if err != nil {
			h.sendError(resp, err)
			return
		}
		logger.Infof("backups download request successful for %q", id)
	default:
		h.sendError(resp, errors.MethodNotAllowedf("unsupported method: %q", req.Method))
	}
}

func (h *backupHandler) download(backups backups.Backups, resp http.ResponseWriter, req *http.Request) (string, error) {
	args, err := h.parseGETArgs(req)
	if err != nil {
		return "", err
	}
	logger.Infof("backups download request for %q", args.ID)

	meta, archive, err := backups.Get(args.ID)
	if err != nil {
		return "", err
	}
	defer archive.Close()

	err = h.sendFile(archive, meta.Checksum(), resp)
	return args.ID, err
}

func (h *backupHandler) read(req *http.Request, expectedType string) ([]byte, error) {
	defer req.Body.Close()

	ctype := req.Header.Get("Content-Type")
	if ctype != expectedType {
		return nil, errors.Errorf("expected Content-Type %q, got %q", expectedType, ctype)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, errors.Annotate(err, "while reading request body")
	}

	return body, nil
}

func (h *backupHandler) parseGETArgs(req *http.Request) (*params.BackupsDownloadArgs, error) {
	body, err := h.read(req, params.ContentTypeJSON)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var args params.BackupsDownloadArgs
	if err := json.Unmarshal(body, &args); err != nil {
		return nil, errors.Annotate(err, "while de-serializing args")
	}

	return &args, nil
}

func (h *backupHandler) sendFile(file io.Reader, checksum string, resp http.ResponseWriter) error {
	// We don't set the Content-Length header, leaving it at -1.
	resp.Header().Set("Content-Type", params.ContentTypeRaw)
	resp.Header().Set("Digest", params.EncodeChecksum(checksum))
	resp.WriteHeader(http.StatusOK)
	if _, err := io.Copy(resp, file); err != nil {
		return errors.Annotate(err, "while streaming archive")
	}
	return nil
}

// sendError sends a JSON-encoded error response.
// Note the difference from the error response sent by
// the sendError function - the error is encoded directly
// rather than in the Error field.
func (h *backupHandler) sendError(w http.ResponseWriter, err error) {
	err, status := apiservererrors.ServerErrorAndStatus(err)
	if err := sendStatusAndJSON(w, status, err); err != nil {
		logger.Errorf("%v", err)
	}
}
