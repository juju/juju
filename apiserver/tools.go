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
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	envtools "github.com/juju/juju/environs/tools"
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
	if errors.IsNotFound(err) {
		// Tools could not be found in toolstorage,
		// so look for them in simplestreams, fetch
		// them and cache in toolstorage.
		reader, err = h.fetchAndCacheTools(version, storage)
	}
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read tools tarball")
	}
	return data, nil
}

// fetchAndCacheTools fetches tools with the specified version by searching for a URL
// in simplestreams and GETting it, caching the result in toolstorage before returning
// to the caller.
func (h *toolsDownloadHandler) fetchAndCacheTools(v version.Binary, stor toolstorage.Storage) (io.ReadCloser, error) {
	envcfg, err := h.state.EnvironConfig()
	if err != nil {
		return nil, err
	}
	env, err := environs.New(envcfg)
	if err != nil {
		return nil, err
	}
	tools, err := envtools.FindExactTools(env, v.Number, v.Series, v.Arch)
	if err != nil {
		return nil, err
	}

	// No need to verify the server's identity because we verify the SHA-256 hash.
	resp, err := utils.GetNonValidatingHTTPClient().Get(tools.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("bad HTTP response: %v", resp.Status)
	}
	data, sha256, err := readAndHash(resp.Body)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != tools.Size {
		return nil, errors.Errorf("size mismatch for %s", tools.URL)
	}
	if sha256 != tools.SHA256 {
		return nil, errors.Errorf("hash mismatch for %s", tools.URL)
	}

	// Cache tarball in toolstorage before returning.
	metadata := toolstorage.Metadata{
		Version: v,
		Size:    tools.Size,
		SHA256:  tools.SHA256,
	}
	if err := stor.AddTools(bytes.NewReader(data), metadata); err != nil {
		return nil, errors.Annotate(err, "error caching tools")
	}
	return ioutil.NopCloser(bytes.NewReader(data)), nil
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

	// Make sure the content type is x-tar-gz.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/x-tar-gz" {
		return nil, errors.Errorf("expected Content-Type: application/x-tar-gz, got: %v", contentType)
	}

	// Get the server root, so we know how to form the URL in the Tools returned.
	serverRoot, err := h.getServerRoot(r, query)
	if err != nil {
		return nil, errors.Annotate(err, "cannot to determine server root")
	}

	// We'll clone the tools for each additional series specified.
	cloneSeries := strings.Split(query.Get("series"), ",")
	logger.Debugf("request to upload tools: %s", toolsVersion)
	logger.Debugf("additional series: %s", cloneSeries)

	toolsVersions := []version.Binary{toolsVersion}
	for _, series := range cloneSeries {
		if series != toolsVersion.Series {
			v := toolsVersion
			v.Series = series
			toolsVersions = append(toolsVersions, v)
		}
	}
	return h.handleUpload(r.Body, toolsVersions, serverRoot)
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
	return fmt.Sprintf("https://%s/environment/%s", r.Host, uuid), nil
}

// handleUpload uploads the tools data from the reader to env storage as the specified version.
func (h *toolsUploadHandler) handleUpload(r io.Reader, toolsVersions []version.Binary, serverRoot string) (*tools.Tools, error) {
	storage, err := h.state.ToolsStorage()
	if err != nil {
		return nil, err
	}
	defer storage.Close()

	// Read the tools tarball from the request, calculating the sha256 along the way.
	data, sha256, err := readAndHash(r)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("no tools uploaded")
	}

	// TODO(wallyworld): check integrity of tools tarball.

	// Store tools and metadata in toolstorage.
	for _, v := range toolsVersions {
		metadata := toolstorage.Metadata{
			Version: v,
			Size:    int64(len(data)),
			SHA256:  sha256,
		}
		logger.Debugf("uploading tools %+v to storage", metadata)
		if err := storage.AddTools(bytes.NewReader(data), metadata); err != nil {
			return nil, err
		}
	}

	tools := &tools.Tools{
		Version: toolsVersions[0],
		Size:    int64(len(data)),
		SHA256:  sha256,
		URL:     common.ToolsURL(serverRoot, toolsVersions[0]),
	}
	return tools, nil
}

func readAndHash(r io.Reader) (data []byte, sha256hex string, err error) {
	hash := sha256.New()
	data, err = ioutil.ReadAll(io.TeeReader(r, hash))
	if err != nil {
		return nil, "", errors.Annotate(err, "error processing file upload")
	}
	return data, fmt.Sprintf("%x", hash.Sum(nil)), nil
}
