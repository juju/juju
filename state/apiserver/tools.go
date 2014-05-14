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
	"strings"

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
		h.authError(w, h)
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
	var fakeSeries []string
	seriesParam := query.Get("series")
	if seriesParam != "" {
		fakeSeries = strings.Split(seriesParam, ",")
	}
	logger.Debugf("request to upload tools %s for series %q", toolsVersion, seriesParam)
	// Make sure the content type is x-tar-gz.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/x-tar-gz" {
		return nil, false, fmt.Errorf("expected Content-Type: application/x-tar-gz, got: %v", contentType)
	}
	return h.handleUpload(r.Body, toolsVersion, fakeSeries...)
}

// handleUpload uploads the tools data from the reader to env storage as the specified version.
func (h *toolsHandler) handleUpload(r io.Reader, toolsVersion version.Binary, fakeSeries ...string) (*tools.Tools, bool, error) {
	// Set up a local temp directory for the tools tarball.
	tmpDir, err := ioutil.TempDir("", "juju-upload-tools-")
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
	logger.Debugf("saving uploaded tools to temp file: %s", fullToolsFilename)
	defer toolsFile.Close()
	sha256hash := sha256.New()
	var size int64
	if size, err = io.Copy(toolsFile, io.TeeReader(r, sha256hash)); err != nil {
		return nil, false, fmt.Errorf("error processing file upload: %v", err)
	}
	if size == 0 {
		return nil, false, fmt.Errorf("no tools uploaded")
	}

	// TODO(wallyworld): check integrity of tools tarball.

	// Create a tools record and sync to storage.
	uploadedTools := &tools.Tools{
		Version: toolsVersion,
		Size:    size,
		SHA256:  fmt.Sprintf("%x", sha256hash.Sum(nil)),
	}
	logger.Debugf("about to upload tools %+v to storage", uploadedTools)
	return h.uploadToStorage(uploadedTools, tmpDir, toolsFilename, fakeSeries...)
}

// uploadToStorage uploads the tools from the specified directory to environment storage.
func (h *toolsHandler) uploadToStorage(uploadedTools *tools.Tools, toolsDir,
	toolsFilename string, fakeSeries ...string) (*tools.Tools, bool, error) {

	// SyncTools requires simplestreams metadata to find the tools to upload.
	stor, err := filestorage.NewFileStorageWriter(toolsDir)
	if err != nil {
		return nil, false, fmt.Errorf("cannot create metadata storage: %v", err)
	}
	// Generate metadata for the fake series. The URL for each fake series
	// record points to the same tools tarball.
	allToolsMetadata := []*tools.Tools{uploadedTools}
	for _, series := range fakeSeries {
		vers := uploadedTools.Version
		vers.Series = series
		allToolsMetadata = append(allToolsMetadata, &tools.Tools{
			Version: vers,
			URL:     uploadedTools.URL,
			Size:    uploadedTools.Size,
			SHA256:  uploadedTools.SHA256,
		})
	}
	err = envtools.MergeAndWriteMetadata(stor, allToolsMetadata, false)
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
	builtTools := &sync.BuiltTools{
		Version:     uploadedTools.Version,
		Dir:         toolsDir,
		StorageName: toolsFilename,
		Size:        uploadedTools.Size,
		Sha256Hash:  uploadedTools.SHA256,
	}
	uploadedTools, err = sync.SyncBuiltTools(env.Storage(), builtTools, fakeSeries...)
	if err != nil {
		return nil, false, err
	}
	return uploadedTools, !envConfig.SSLHostnameVerification(), nil
}
