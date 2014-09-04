// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/toolstorage"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// toolsHandler is the base type for uploading and downloading
// tools over HTTPS via the API server.
type toolsHandler struct {
	httpHandler
}

// toolsHandler handles tool upload through HTTPS in the API server.
type toolsUploadHandler struct {
	toolsHandler
}

// toolsHandler handles tool download through HTTPS in the API server.
type toolsDownloadHandler struct {
	toolsHandler
}

func (h *toolsDownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.validateEnvironUUID(r); err != nil {
		h.sendError(w, http.StatusNotFound, err.Error())
		return
	}

	switch r.Method {
	case "GET":
		tarball, err := h.processGet(r)
		if err != nil {
			h.sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.sendTools(w, http.StatusOK, tarball)
	default:
		h.sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", r.Method))
	}
}

func (h *toolsUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.authenticate(r); err != nil {
		h.authError(w, h)
		return
	}

	if err := h.validateEnvironUUID(r); err != nil {
		h.sendError(w, http.StatusNotFound, err.Error())
		return
	}

	switch r.Method {
	case "POST":
		// Add tools to storage.
		agentTools, err := h.processPost(r)
		if err != nil {
			h.sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.sendJSON(w, http.StatusOK, &params.ToolsResult{Tools: agentTools})
	default:
		h.sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", r.Method))
	}
}

// sendJSON sends a JSON-encoded response to the client.
func (h *toolsHandler) sendJSON(w http.ResponseWriter, statusCode int, response *params.ToolsResult) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	w.Write(body)
	return nil
}

// sendError sends a JSON-encoded error response.
func (h *toolsHandler) sendError(w http.ResponseWriter, statusCode int, message string) {
	err := common.ServerError(errors.New(message))
	if err := h.sendJSON(w, statusCode, &params.ToolsResult{Error: err}); err != nil {
		logger.Errorf("failed to send error: %v", err)
	}
}

// processGet handles a tools GET request.
func (h *toolsDownloadHandler) processGet(r *http.Request) ([]byte, error) {
	version, err := version.ParseBinary(r.URL.Query().Get(":version"))
	if err != nil {
		return nil, err
	}
	storage, err := h.state.ToolsStorage()
	if err != nil {
		return nil, err
	}
	defer storage.Close()
	_, reader, err := storage.Tools(version)
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read tools tarball")
	}
	return data, nil
}

// sendTools streams the tools tarball to the client.
func (h *toolsDownloadHandler) sendTools(w http.ResponseWriter, statusCode int, tarball []byte) {
	w.Header().Set("Content-Type", "application/x-tar-gz")
	w.Header().Set("Content-Length", fmt.Sprint(len(tarball)))
	w.WriteHeader(statusCode)
	if _, err := w.Write(tarball); err != nil {
		h.sendError(w, http.StatusBadRequest, fmt.Sprintf("failed to write tools: %v", err))
		return
	}
}

// processPost handles a tools upload POST request after authentication.
func (h *toolsUploadHandler) processPost(r *http.Request) (*tools.Tools, error) {
	query := r.URL.Query()
	binaryVersionParam := query.Get("binaryVersion")
	if binaryVersionParam == "" {
		return nil, errors.New("expected binaryVersion argument")
	}
	toolsVersion, err := version.ParseBinary(binaryVersionParam)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid tools version %q", binaryVersionParam)
	}
	logger.Debugf("request to upload tools %s", toolsVersion)
	// Make sure the content type is x-tar-gz.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/x-tar-gz" {
		return nil, errors.Errorf("expected Content-Type: application/x-tar-gz, got: %v", contentType)
	}
	serverRoot, err := h.getServerRoot(r, query)
	if err != nil {
		return nil, errors.Annotate(err, "cannot to determine server root")
	}
	return h.handleUpload(r.Body, toolsVersion, serverRoot)
}

func (h *toolsUploadHandler) getServerRoot(r *http.Request, query url.Values) (string, error) {
	uuid := query.Get(":envuuid")
	if uuid == "" {
		env, err := h.state.Environment()
		if err != nil {
			return "", err
		}
		uuid = env.UUID()
	}
	return fmt.Sprintf("https://%s/environment/%s", r.RemoteAddr, uuid), nil
}

// handleUpload uploads the tools data from the reader to env storage as the specified version.
func (h *toolsUploadHandler) handleUpload(r io.Reader, toolsVersion version.Binary, serverRoot string) (*tools.Tools, error) {
	storage, err := h.state.ToolsStorage()
	if err != nil {
		return nil, err
	}
	defer storage.Close()

	// Read the tools tarball from the request, calculating the sha256 along the way.
	sha256hash := sha256.New()
	var buf bytes.Buffer
	var size int64
	size, err = io.Copy(io.MultiWriter(&buf, sha256hash), r)
	if err != nil {
		return nil, errors.Annotate(err, "error processing file upload")
	}
	if size == 0 {
		return nil, errors.New("no tools uploaded")
	}

	// TODO(wallyworld): check integrity of tools tarball.

	// Store tools and metadata in state.
	metadata := toolstorage.Metadata{
		Version: toolsVersion,
		Size:    size,
		SHA256:  fmt.Sprintf("%x", sha256hash.Sum(nil)),
	}
	logger.Debugf("uploading tools %+v to storage", metadata)
	if err := storage.AddTools(&buf, metadata); err != nil {
		return nil, err
	}

	// TODO(axw) duplicate tools iff uploading locally built tools.
	osSeries := version.OSSupportedSeries(metadata.Version.OS)
	for _, series := range osSeries {
		if series == metadata.Version.Series {
			continue
		}
		v := metadata.Version
		v.Series = series
		err := storage.AddToolsAlias(v, metadata.Version)
		if err != nil && !errors.IsAlreadyExists(err) {
			return nil, err
		}
	}

	tools := &tools.Tools{
		Version: metadata.Version,
		Size:    metadata.Size,
		SHA256:  metadata.SHA256,
		URL:     common.ToolsURL(serverRoot, metadata.Version),
	}
	return tools, nil
}
