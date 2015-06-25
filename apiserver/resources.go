// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmresources"
	"github.com/juju/juju/state"
)

// resourcesHandler is the base type for uploading and downloading
// resources over HTTPS via the API server.
type resourcesHandler struct {
	httpHandler
}

// resourcesUploadHandler handles resources upload through HTTPS in the API server.
type resourcesUploadHandler struct {
	resourcesHandler
}

// resourcesHandler handles resources download through HTTPS in the API server.
type resourcesDownloadHandler struct {
	resourcesHandler
}

func (h *resourcesDownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	stateWrapper, err := h.validateEnvironUUID(r)
	if err != nil {
		h.sendExistingError(w, http.StatusNotFound, err)
		return
	}
	defer stateWrapper.cleanup()

	switch r.Method {
	case "GET":
		readers, err := h.processGet(r, getResourceState(stateWrapper.state))
		if err != nil {
			logger.Errorf("GET(%s) failed: %v", r.URL, err)
			h.sendExistingError(w, http.StatusBadRequest, err)
			return
		}
		err = h.sendResources(w, http.StatusOK, readers)
		if err != nil {
			logger.Errorf("GET(%s) failed: %v", r.URL, err)
			h.sendExistingError(w, http.StatusInternalServerError, err)
			return
		}
	default:
		h.sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", r.Method))
	}
}

func (h *resourcesUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Validate before authenticate because the authentication is dependent
	// on the state connection that is determined during the validation.
	stateWrapper, err := h.validateEnvironUUID(r)
	if err != nil {
		h.sendExistingError(w, http.StatusNotFound, err)
		return
	}
	defer stateWrapper.cleanup()

	if err := stateWrapper.authenticateUser(r); err != nil {
		h.authError(w, h)
		return
	}

	switch r.Method {
	case "POST":
		// Add resources to storage.
		resource, err := h.processPost(r, getResourceState(stateWrapper.state))
		if err != nil {
			h.sendExistingError(w, http.StatusBadRequest, err)
			return
		}
		h.sendJSON(w, http.StatusOK, &params.ResourceResult{Resource: *resource})
	default:
		h.sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", r.Method))
	}
}

// sendJSON sends a JSON-encoded response to the client.
func (h *resourcesHandler) sendJSON(w http.ResponseWriter, statusCode int, response *params.ResourceResult) error {
	w.Header().Set("Content-Type", apihttp.CTypeJSON)
	w.WriteHeader(statusCode)
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	w.Write(body)
	return nil
}

// sendError sends a JSON-encoded error response using desired
// error message.
func (h *resourcesHandler) sendError(w http.ResponseWriter, statusCode int, message string) {
	h.sendExistingError(w, statusCode, errors.New(message))
}

// sendExistingError sends a JSON-encoded error response
// for errors encountered during processing.
func (h *resourcesHandler) sendExistingError(w http.ResponseWriter, statusCode int, existing error) {
	logger.Debugf("sending error: %v %v", statusCode, existing)
	err := common.ServerError(existing)
	if err := h.sendJSON(w, statusCode, &params.ResourceResult{Error: err}); err != nil {
		logger.Errorf("failed to send error: %v", err)
	}
}

func resourceAttributes(url *url.URL) charmresources.ResourceAttributes {
	query := url.Query()
	attrs := charmresources.ResourceAttributes{
		PathName: query.Get("path"),
		Revision: query.Get("revision"),
		Type:     query.Get("type"),
		User:     query.Get("user"),
		Org:      query.Get("org"),
		Stream:   query.Get("stream"),
		Series:   query.Get("series"),
	}
	// If this is for a download then path is part of URL.
	if attrs.PathName == "" {
		attrs.PathName = query.Get(":basepath")
	}
	return attrs
}

func resourceURL(serverRoot string, attrs charmresources.ResourceAttributes) string {
	url := fmt.Sprintf("%s/resources/%s", serverRoot, attrs.PathName)
	url += fmt.Sprintf("?revision=%s", attrs.Revision)
	if attrs.Org != "" {
		url += fmt.Sprintf("&org=%s", attrs.Org)
	}
	if attrs.User != "" {
		url += fmt.Sprintf("&user=%s", attrs.User)
	}
	if attrs.Stream != "" {
		url += fmt.Sprintf("&stream=%s", attrs.Stream)
	}
	if attrs.Series != "" {
		url += fmt.Sprintf("&series=%s", attrs.Series)
	}
	if attrs.Type != "" {
		url += fmt.Sprintf("&type=%s", attrs.Type)
	}
	return url
}

// processGet handles a resource GET request.
func (h *resourcesDownloadHandler) processGet(r *http.Request, st ResourceState) ([]charmresources.ResourceReader, error) {
	attrs := resourceAttributes(r.URL)
	resourcePath, err := charmresources.ResourcePath(attrs)
	if err != nil {
		return nil, errors.Annotate(err, "invalid resource attributes")
	}
	manager := st.ResourceManager()
	readers, err := manager.ResourceGet(resourcePath)
	if errors.IsNotFound(err) {
		// TODO(wallyworld) - fetch from public store and cache
	}
	if err != nil {
		return nil, err
	}
	if len(readers) > 1 {
		// TODO(wallyworld) - support multiple resource streams
		return nil, errors.NotSupportedf("dependent resources")
	}
	return readers, nil
}

// sendResources streams the resources to the client.
func (h *resourcesDownloadHandler) sendResources(w http.ResponseWriter, statusCode int, readers []charmresources.ResourceReader) error {
	if len(readers) != 1 {
		// TODO(wallyworld) - support dependent resources (multipart)
		// This has already been checked.
		panic("dependent resources not supported")
	}
	reader := readers[0]
	defer reader.Close()
	// Stream the resource to the caller.
	logger.Debugf("streaming resource %v from state resource store", reader.Path)
	w.Header().Set("Content-Type", apihttp.CTypeRaw)
	w.Header().Set("Digest", fmt.Sprintf("%s=%s", apihttp.DigestSHA, reader.SHA384Hash))
	w.Header().Set("Content-Length", fmt.Sprint(reader.Size))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, reader); err != nil {
		return errors.Annotate(err, "while streaming resource")
	}
	return nil
}

// processPost handles a resource upload POST request after authentication.
func (h *resourcesUploadHandler) processPost(r *http.Request, st ResourceState) (*params.ResourceMetadata, error) {
	attrs := resourceAttributes(r.URL)
	if attrs.Revision == "" {
		return nil, errors.New("revision is required to upload resources")
	}
	// Make sure the content type is correct.
	contentType := r.Header.Get("Content-Type")
	if contentType != apihttp.CTypeRaw {
		return nil, errors.Errorf("expected Content-Type: %v, got: %v", apihttp.CTypeRaw, contentType)
	}

	// Get the server root, so we know how to form the URL in the resources returned.
	serverRoot, err := h.getServerRoot(r, r.URL.Query(), st)
	if err != nil {
		return nil, errors.Annotate(err, "cannot to determine server root")
	}

	resourcePath, err := charmresources.ResourcePath(attrs)
	if err != nil {
		return nil, errors.Annotate(err, "invalid resource attributes")
	}
	query := r.URL.Query()
	resourceMetadata := charmresources.Resource{
		Path:       resourcePath,
		SHA384Hash: query.Get("sha384"),
	}

	defer r.Body.Close()
	return h.handleUpload(r.Body, attrs, resourceMetadata, serverRoot, st)
}

func (h *resourcesUploadHandler) getServerRoot(r *http.Request, query url.Values, st ResourceState) (string, error) {
	uuid := query.Get(":envuuid")
	if uuid == "" {
		uuid = st.EnvironUUID()
	}
	return fmt.Sprintf("https://%s/environment/%s", r.Host, uuid), nil
}

// handleUpload uploads the RESOURCES data from the reader to resource storage at the specified path.
func (h *resourcesUploadHandler) handleUpload(
	rdr io.Reader, attrs charmresources.ResourceAttributes, resourceMetadata charmresources.Resource,
	serverRoot string, st ResourceState,
) (*params.ResourceMetadata, error) {
	// Check if changes are allowed and the command may proceed.
	blockChecker := common.NewBlockChecker(st)
	if err := blockChecker.ChangeAllowed(); err != nil {
		return nil, errors.Trace(err)
	}
	manager := st.ResourceManager()
	metadata, err := manager.ResourcePut(resourceMetadata, rdr)
	if err != nil {
		return nil, errors.Annotatef(err, "saving resource %v", metadata.Path)
	}
	if metadata.Size == 0 {
		return nil, errors.New("no resources uploaded")
	}
	path := metadata.Path
	if path[0] == '/' {
		path = path[1:]
	}
	result := &params.ResourceMetadata{
		ResourcePath: metadata.Path,
		Size:         metadata.Size,
		SHA384:       metadata.SHA384Hash,
		Created:      metadata.Created,
		URL:          resourceURL(serverRoot, attrs),
	}
	return result, nil
}

// ResourceState is used to allow a mock to be substituted when testing.
type ResourceState interface {
	// ResourceManager provides the capability to persist resources.
	ResourceManager() charmresources.ResourceManager

	// EnvironUUID is needed for resource uploads.
	EnvironUUID() string

	// GetBlockForType is required to block operations.
	GetBlockForType(t state.BlockType) (state.Block, bool, error)
}

type stateShim struct {
	*state.State
}

var getResourceState = func(st *state.State) ResourceState {
	return stateShim{st}
}
