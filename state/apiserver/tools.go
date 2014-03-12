// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/sync"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

// toolsHandler handles tool upload through HTTPS in the API server.
type toolsHandler struct {
	httpHandler
}

func (h *toolsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.authenticate(r); err != nil {
		h.authError(w)
		return
	}

	switch r.Method {
	case "POST":
		// Add a local charm to the store provider.
		// Requires a "series" query specifying the series to use for the charm.
		agentTools, disableSSLHostnameVerification, err := h.processPost(r)
		if err != nil {
			h.sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.sendJSON(w, http.StatusOK, &params.ToolsResult{
			Tools: agentTools,
			DisableSSLHostnameVerification: disableSSLHostnameVerification,
		})
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
func (h *toolsHandler) sendError(w http.ResponseWriter, statusCode int, message string) error {
	err := common.ServerError(fmt.Errorf(message))
	return h.sendJSON(w, statusCode, &params.ToolsResult{Error: err})
}

// processPost handles a charm upload POST request after authentication.
func (h *toolsHandler) processPost(r *http.Request) (*tools.Tools, bool, error) {
	query := r.URL.Query()
	binaryVersionParam := query.Get("binaryVersion")
	if binaryVersionParam == "" {
		return nil, false, fmt.Errorf("expected binaryVersion argument")
	}
	toolsVersion, err := version.ParseBinary(binaryVersionParam)
	if err != nil {
		return nil, false, fmt.Errorf("invalid tools version %q: %v", binaryVersionParam, err)
	}
	// Make sure the content type is x-tar-gz.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/x-tar-gz" {
		return nil, false, fmt.Errorf("expected Content-Type: application/x-tar-gz, got: %v", contentType)
	}
	return h.handleUpload(r.Body, toolsVersion)
}

// handleUpload uploads the tools data from the reader to env storage as the specified version.
func (h *toolsHandler) handleUpload(r io.Reader, toolsVersion version.Binary) (*tools.Tools, bool, error) {
	// Set up a local temp directory for the tools tarball.
	tmpDir, err := ioutil.TempDir("", "uploadtools-")
	if err != nil {
		return nil, false, fmt.Errorf("cannot create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	toolsFilename := envtools.StorageName(toolsVersion)
	toolsDir := path.Dir(toolsFilename)
	fullToolsDir := path.Join(tmpDir, toolsDir)
	err = os.MkdirAll(fullToolsDir, 0700)
	if err != nil {
		return nil, false, fmt.Errorf("cannot create tools dir %s: %v", toolsDir, err)
	}

	// Read the tools tarball from the request, calculating the sha256 along the way.
	fullToolsFilename := path.Join(tmpDir, toolsFilename)
	toolsFile, err := os.Create(fullToolsFilename)
	if err != nil {
		return nil, false, fmt.Errorf("cannot create tools file %s: %v", fullToolsFilename, err)
	}
	defer toolsFile.Close()
	sha256hash := sha256.New()
	var size int64
	if size, err = io.Copy(toolsFile, io.TeeReader(r, sha256hash)); err != nil {
		return nil, false, fmt.Errorf("error processing file upload: %v", err)
	}
	if size == 0 {
		return nil, false, fmt.Errorf("no tools uploaded")
	}

	// Create a tools record and sync to storage.
	uploadedTools := &tools.Tools{
		Version: toolsVersion,
		Size:    size,
		SHA256:  fmt.Sprintf("%x", sha256hash.Sum(nil)),
	}
	return h.uploadToStorage(uploadedTools, tmpDir, toolsFilename)
}

// uploadToStorage uploads the tools from the specified directory to environment storage.
func (h *toolsHandler) uploadToStorage(uploadedTools *tools.Tools, toolsDir,
	toolsFilename string) (*tools.Tools, bool, error) {

	// SyncTools requires simplestreams metadata to find the tools to upload.
	stor, err := filestorage.NewFileStorageWriter(toolsDir)
	if err != nil {
		return nil, false, fmt.Errorf("cannot create metadata storage: %v", err)
	}
	err = envtools.MergeAndWriteMetadata(stor, []*tools.Tools{uploadedTools}, false)
	if err != nil {
		return nil, false, fmt.Errorf("cannot get environment config: %v", err)
	}

	// Create the environment so we can get the storage to which we upload the tools.
	envConfig, err := h.state.EnvironConfig()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get environment config: %v", err)
	}
	env, err := environs.New(envConfig)
	if err != nil {
		return nil, false, fmt.Errorf("cannot access environment: %v", err)
	}

	// Now perform the upload.
	url, err := syncTools(env, toolsDir, toolsFilename)
	if err != nil {
		return nil, false, err
	}
	uploadedTools.URL = url
	return uploadedTools, !envConfig.SSLHostnameVerification(), nil
}

// syncTools invokes the SyncTools API to upload tools to environment storage.
func syncTools(env environs.Environ, toolsDir, toolsFilename string) (string, error) {
	stor := env.Storage()
	sctx := &sync.SyncContext{
		Target:      stor,
		Source:      toolsDir,
		AllVersions: true,
	}
	if err := sync.SyncTools(sctx); err != nil {
		return "", err
	}
	url, err := stor.URL(toolsFilename)
	if err != nil {
		return "", err
	}
	return url, nil
}
