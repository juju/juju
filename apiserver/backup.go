// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/juju/errors"

	backupsAPI "github.com/juju/juju/apiserver/backups"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
)

// TODO(ericsnow) This code should be in the backups package.

var newBackups = func(st *state.State) (backups.Backups, error) {
	return backupsAPI.NewBackups(st)
}

// backupHandler handles backup requests
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
		logger.Infof("handling backups download request")
		args, err := h.parseGETArgs(req)
		if err != nil {
			h.sendError(resp, http.StatusInternalServerError, err.Error())
			return
		}
		logger.Infof("backups download request for %q", args.ID)

		meta, archive, err := backups.Get(args.ID)
		if err != nil {
			h.sendError(resp, http.StatusInternalServerError, err.Error())
			return
		}
		defer archive.Close()

		err = h.sendFile(archive, meta.Checksum(), "SHA", resp)
		if err != nil {
			h.sendError(resp, http.StatusInternalServerError, err.Error())
			return
		}
		logger.Infof("backups download request successful for %q", args.ID)
	case "PUT":
		defer req.Body.Close()
		logger.Infof("handling backups upload request")

		archive, metaResult, err := h.extractUploadArgs(req)
		if err != nil {
			h.sendError(resp, http.StatusInternalServerError, err.Error())
			return
		}

		meta, err := metaResult.AsMetadata()
		if err != nil {
			h.sendError(resp, http.StatusInternalServerError, err.Error())
			return
		}

		id, err := backups.Add(archive, *meta)
		if err != nil {
			h.sendError(resp, http.StatusInternalServerError, err.Error())
			return
		}

		h.sendJSON(resp, http.StatusOK, &params.BackupsUploadResult{ID: id})
		logger.Infof("backups upload request successful for %q", id)
	default:
		h.sendError(resp, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", req.Method))
	}
}

type getter interface {
	Get(string) string
}

func (h *backupHandler) checkContentType(header getter, expected string) error {
	ctype := header.Get("Content-Type")
	if ctype != expected {
		return errors.Errorf("expected Content-Type %q, got %q", expected, ctype)
	}
	return nil
}

func (h *backupHandler) extractUploadArgs(req *http.Request) (io.ReadCloser, *params.BackupsMetadataResult, error) {
	ctype := req.Header.Get("Content-Type")
	mediaType, cParams, err := mime.ParseMediaType(ctype)
	if err != nil {
		return nil, nil, errors.Annotate(err, "while parsing content type header")
	}

	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, nil, errors.Errorf("expected multipart Content-Type, got %q", mediaType)
	}
	reader := multipart.NewReader(req.Body, cParams["boundary"])

	// Extract the metadata.
	part, err := reader.NextPart()
	if err != nil {
		if err == io.EOF {
			return nil, nil, errors.New("missing metadata")
		}
		return nil, nil, err
	}
	if err := h.checkContentType(part.Header, "application/json"); err != nil {
		return nil, nil, err
	}
	var metaResult params.BackupsMetadataResult
	if err := json.NewDecoder(part).Decode(&metaResult); err != nil {
		return nil, nil, err
	}

	// Extract the archive.
	part, err = reader.NextPart()
	if err != nil {
		if err == io.EOF {
			return nil, nil, errors.New("missing archive")
		}
		return nil, nil, err
	}
	if err := h.checkContentType(part.Header, "application/octet-stream"); err != nil {
		return nil, nil, err
	}
	// We're not going to worry about verifying that the file matches the
	// metadata (e.g. size, checksum).
	archive := part

	// We are going to trust that there aren't any more attachments after
	// the file.  If there are, we ignore them.

	return archive, &metaResult, nil
}

func (h *backupHandler) read(req *http.Request) ([]byte, error) {
	defer req.Body.Close()

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, errors.Annotate(err, "while reading request body")
	}

	return body, nil
}

func (h *backupHandler) parseGETArgs(req *http.Request) (*params.BackupsDownloadArgs, error) {
	if err := h.checkContentType(req.Header, "application/json"); err != nil {
		return nil, errors.Trace(err)
	}
	body, err := h.read(req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var args params.BackupsDownloadArgs
	if err := json.Unmarshal(body, &args); err != nil {
		return nil, errors.Annotate(err, "while de-serializing args")
	}

	return &args, nil
}

func (h *backupHandler) sendFile(file io.Reader, checksum, algorithm string, resp http.ResponseWriter) error {
	// We don't set the Content-Length header, leaving it at -1.
	resp.Header().Set("Content-Type", "application/octet-stream")
	resp.Header().Set("Digest", fmt.Sprintf("%s=%s", algorithm, checksum))
	resp.WriteHeader(http.StatusOK)
	if _, err := io.Copy(resp, file); err != nil {
		return errors.Annotate(err, "while streaming archive")
	}
	return nil
}

// sendJSON sends a JSON-encoded result.
func (h *backupHandler) sendJSON(w http.ResponseWriter, statusCode int, result interface{}) {
	body, err := json.Marshal(result)
	if err != nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(body)

	logger.Infof("backups request successful")
}

// sendError sends a JSON-encoded error response.
func (h *backupHandler) sendError(w http.ResponseWriter, statusCode int, message string) {
	failure := params.Error{
		Message: message,
		// Leave Code empty.
	}

	h.sendJSON(w, statusCode, &failure)
}
