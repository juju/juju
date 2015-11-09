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

	apiserverbackups "github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/httpattachment"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
)

var newBackups = func(st *state.State) (backups.Backups, io.Closer) {
	stor := backups.NewStorage(st)
	return backups.NewBackups(stor), stor
}

// backupHandler handles backup requests.
type backupHandler struct {
	ctxt httpContext
}

func (h *backupHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	// Validate before authenticate because the authentication is dependent
	// on the state connection that is determined during the validation.
	st, _, err := h.ctxt.stateForRequestAuthenticatedUser(req)
	if err != nil {
		h.sendError(resp, err)
		return
	}

	backups, closer := newBackups(st)
	defer closer.Close()

	switch req.Method {
	case "GET":
		logger.Infof("handling backups download request")
		id, err := h.download(backups, resp, req)
		if err != nil {
			h.sendError(resp, err)
			return
		}
		logger.Infof("backups download request successful for %q", id)
	case "PUT":
		logger.Infof("handling backups upload request")
		id, err := h.upload(backups, resp, req)
		if err != nil {
			h.sendError(resp, err)
			return
		}
		logger.Infof("backups upload request successful for %q", id)
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

func (h *backupHandler) upload(backups backups.Backups, resp http.ResponseWriter, req *http.Request) (string, error) {
	// Since we want to stream the archive in we cannot simply use
	// mime/multipart directly.
	defer req.Body.Close()

	var metaResult params.BackupsMetadataResult
	archive, err := httpattachment.Get(req, &metaResult)
	if err != nil {
		return "", err
	}

	if err := validateBackupMetadataResult(metaResult); err != nil {
		return "", err
	}

	meta := apiserverbackups.MetadataFromResult(metaResult)
	id, err := backups.Add(archive, meta)
	if err != nil {
		return "", err
	}

	sendStatusAndJSON(resp, http.StatusOK, &params.BackupsUploadResult{ID: id})
	return id, nil
}

func validateBackupMetadataResult(metaResult params.BackupsMetadataResult) error {
	if metaResult.ID != "" {
		return errors.New("got unexpected metadata ID")
	}
	if !metaResult.Stored.IsZero() {
		return errors.New(`got unexpected metadata "Stored" value`)
	}
	return nil
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
	resp.Header().Set("Digest", fmt.Sprintf("%s=%s", params.DigestSHA, checksum))
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
	err, status := common.ServerErrorAndStatus(err)

	sendStatusAndJSON(w, status, err)
}
