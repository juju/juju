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

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/sync"
	envtools "github.com/juju/juju/environs/tools"
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
		tools, verifyHostname, err := h.processGet(r)
		if err != nil {
			h.sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.sendTools(w, http.StatusOK, tools, verifyHostname)
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
func (h *toolsHandler) sendError(w http.ResponseWriter, statusCode int, message string) {
	err := common.ServerError(errors.New(message))
	if err := h.sendJSON(w, statusCode, &params.ToolsResult{Error: err}); err != nil {
		logger.Errorf("failed to send error: %v", err)
	}
}

// processGet handles a tools GET request.
func (h *toolsDownloadHandler) processGet(r *http.Request) (*tools.Tools, utils.SSLHostnameVerification, error) {
	version, err := version.ParseBinary(r.URL.Query().Get(":version"))
	if err != nil {
		return nil, false, err
	}
	cfg, err := h.state.EnvironConfig()
	if err != nil {
		return nil, false, err
	}
	env, err := environs.New(cfg)
	if err != nil {
		return nil, false, err
	}
	filter := tools.Filter{
		Number: version.Number,
		Series: version.Series,
		Arch:   version.Arch,
	}
	tools, err := envtools.FindTools(env, version.Major, version.Minor, filter, false)
	if err != nil {
		return nil, false, errors.Annotate(err, "failed to find tools")
	}
	verify := utils.SSLHostnameVerification(cfg.SSLHostnameVerification())
	return tools[0], verify, nil
}

// sendTools streams the tools tarball to the client.
func (h *toolsDownloadHandler) sendTools(w http.ResponseWriter, statusCode int, tools *tools.Tools, verify utils.SSLHostnameVerification) {
	client := utils.GetHTTPClient(verify)
	resp, err := client.Get(tools.URL)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, fmt.Sprintf("failed to get %q: %v", tools.URL, err))
		return
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		h.sendError(w, http.StatusBadRequest, fmt.Sprintf("failed to read tools: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/x-gtar")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.WriteHeader(statusCode)
	if _, err := w.Write(data); err != nil {
		h.sendError(w, http.StatusBadRequest, fmt.Sprintf("failed to write tools: %v", err))
		return
	}
}

// processPost handles a tools upload POST request after authentication.
func (h *toolsUploadHandler) processPost(r *http.Request) (*tools.Tools, bool, error) {
	query := r.URL.Query()
	binaryVersionParam := query.Get("binaryVersion")
	if binaryVersionParam == "" {
		return nil, false, errors.New("expected binaryVersion argument")
	}
	toolsVersion, err := version.ParseBinary(binaryVersionParam)
	if err != nil {
		return nil, false, errors.Annotatef(err, "invalid tools version %q", binaryVersionParam)
	}
	logger.Debugf("request to upload tools %s", toolsVersion)
	// Make sure the content type is x-tar-gz.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/x-tar-gz" {
		return nil, false, errors.Errorf("expected Content-Type: application/x-tar-gz, got: %v", contentType)
	}
	return h.handleUpload(r.Body, toolsVersion)
}

// handleUpload uploads the tools data from the reader to env storage as the specified version.
func (h *toolsUploadHandler) handleUpload(r io.Reader, toolsVersion version.Binary) (*tools.Tools, bool, error) {
	// Set up a local temp directory for the tools tarball.
	tmpDir, err := ioutil.TempDir("", "juju-upload-tools-")
	if err != nil {
		return nil, false, errors.Annotate(err, "cannot create temp dir")
	}
	defer os.RemoveAll(tmpDir)
	toolsFilename := envtools.StorageName(toolsVersion)
	toolsDir := path.Dir(toolsFilename)
	fullToolsDir := path.Join(tmpDir, toolsDir)
	err = os.MkdirAll(fullToolsDir, 0700)
	if err != nil {
		return nil, false, errors.Annotatef(err, "cannot create tools dir %s", toolsDir)
	}

	// Read the tools tarball from the request, calculating the sha256 along the way.
	fullToolsFilename := path.Join(tmpDir, toolsFilename)
	toolsFile, err := os.Create(fullToolsFilename)
	if err != nil {
		return nil, false, errors.Annotatef(err, "cannot create tools file %s", fullToolsFilename)
	}
	logger.Debugf("saving uploaded tools to temp file: %s", fullToolsFilename)
	defer toolsFile.Close()
	sha256hash := sha256.New()
	var size int64
	if size, err = io.Copy(toolsFile, io.TeeReader(r, sha256hash)); err != nil {
		return nil, false, errors.Annotate(err, "error processing file upload")
	}
	if size == 0 {
		return nil, false, errors.New("no tools uploaded")
	}

	// TODO(wallyworld): check integrity of tools tarball.

	// Create a tools record and sync to storage.
	uploadedTools := &tools.Tools{
		Version: toolsVersion,
		Size:    size,
		SHA256:  fmt.Sprintf("%x", sha256hash.Sum(nil)),
	}
	logger.Debugf("about to upload tools %+v to storage", uploadedTools)
	return h.uploadToStorage(uploadedTools, tmpDir, toolsFilename)
}

// uploadToStorage uploads the tools from the specified directory to environment storage.
func (h *toolsUploadHandler) uploadToStorage(uploadedTools *tools.Tools, toolsDir, toolsFilename string) (*tools.Tools, bool, error) {
	// SyncTools requires simplestreams metadata to find the tools to upload.
	stor, err := filestorage.NewFileStorageWriter(toolsDir)
	if err != nil {
		return nil, false, errors.Annotate(err, "cannot create metadata storage")
	}
	// Generate metadata for each series of the same OS as the uploaded tools.
	// The URL for each fake series record points to the same tools tarball.
	allToolsMetadata := []*tools.Tools{uploadedTools}
	osSeries := version.OSSupportedSeries(uploadedTools.Version.OS)
	for _, series := range osSeries {
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
		return nil, false, errors.Annotate(err, "cannot get environment config")
	}

	// Create the environment so we can get the storage to which we upload the tools.
	envConfig, err := h.state.EnvironConfig()
	if err != nil {
		return nil, false, errors.Annotate(err, "cannot get environment config")
	}
	env, err := environs.New(envConfig)
	if err != nil {
		return nil, false, errors.Annotate(err, "cannot access environment")
	}

	// Now perform the upload.
	builtTools := &sync.BuiltTools{
		Version:     uploadedTools.Version,
		Dir:         toolsDir,
		StorageName: toolsFilename,
		Size:        uploadedTools.Size,
		Sha256Hash:  uploadedTools.SHA256,
	}
	uploadedTools, err = sync.SyncBuiltTools(env.Storage(), builtTools, osSeries...)
	if err != nil {
		return nil, false, err
	}
	return uploadedTools, !envConfig.SSLHostnameVerification(), nil
}
