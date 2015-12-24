// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"net/http"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

// HTTPHandler is the HTTP handler for the resources endpoint.
type HTTPHandler struct {
	// Connect opens a connection to state resources.
	Connect func(*http.Request) (state.Resources, error)
}

func (h *HTTPHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	st, err := h.Connect(req)
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

func (h *HTTPHandler) upload(backups backups.Backups, resp http.ResponseWriter, req *http.Request) (string, error) {
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

func (h *HTTPHandler) read(req *http.Request, expectedType string) ([]byte, error) {
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

func (h *HTTPHandler) parseGETArgs(req *http.Request) (*params.BackupsDownloadArgs, error) {
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

func (h *HTTPHandler) sendFile(file io.Reader, checksum string, resp http.ResponseWriter) error {
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
func (h *HTTPHandler) sendError(w http.ResponseWriter, err error) {
	err, status := common.ServerErrorAndStatus(err)

	sendStatusAndJSON(w, status, err)
}
