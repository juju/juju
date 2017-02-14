// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/version"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/tools"
)

// toolsHandler handles tool upload through HTTPS in the API server.
type toolsUploadHandler struct {
	ctxt          httpContext
	stateAuthFunc func(*http.Request) (*state.State, func(), error)
}

// toolsHandler handles tool download through HTTPS in the API server.
type toolsDownloadHandler struct {
	ctxt httpContext
}

func (h *toolsDownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	st, releaser, err := h.ctxt.stateForRequestUnauthenticated(r)
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	defer releaser()

	switch r.Method {
	case "GET":
		tarball, err := h.processGet(r, st)
		if err != nil {
			logger.Errorf("GET(%s) failed: %v", r.URL, err)
			if err := sendError(w, errors.NewBadRequest(err, "")); err != nil {
				logger.Errorf("%v", err)
			}
			return
		}
		if err := h.sendTools(w, http.StatusOK, tarball); err != nil {
			logger.Errorf("%v", err)
		}
	default:
		if err := sendError(w, errors.MethodNotAllowedf("unsupported method: %q", r.Method)); err != nil {
			logger.Errorf("%v", err)
		}
	}
}

func (h *toolsUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Validate before authenticate because the authentication is dependent
	// on the state connection that is determined during the validation.
	st, releaser, err := h.stateAuthFunc(r)
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	defer releaser()

	switch r.Method {
	case "POST":
		// Add tools to storage.
		agentTools, err := h.processPost(r, st)
		if err != nil {
			if err := sendError(w, err); err != nil {
				logger.Errorf("%v", err)
			}
			return
		}
		if err := sendStatusAndJSON(w, http.StatusOK, &params.ToolsResult{
			ToolsList: tools.List{agentTools},
		}); err != nil {
			logger.Errorf("%v", err)
		}
	default:
		if err := sendError(w, errors.MethodNotAllowedf("unsupported method: %q", r.Method)); err != nil {
			logger.Errorf("%v", err)
		}
	}
}

// processGet handles a tools GET request.
func (h *toolsDownloadHandler) processGet(r *http.Request, st *state.State) ([]byte, error) {
	version, err := version.ParseBinary(r.URL.Query().Get(":version"))
	if err != nil {
		return nil, errors.Annotate(err, "error parsing version")
	}
	storage, err := st.ToolsStorage()
	if err != nil {
		return nil, errors.Annotate(err, "error getting tools storage")
	}
	defer storage.Close()
	_, reader, err := storage.Open(version.String())
	if errors.IsNotFound(err) {
		// Tools could not be found in tools storage,
		// so look for them in simplestreams, fetch
		// them and cache in tools storage.
		logger.Infof("%v tools not found locally, fetching", version)
		reader, err = h.fetchAndCacheTools(version, storage, st)
		if err != nil {
			err = errors.Annotate(err, "error fetching tools")
		}
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
// in simplestreams and GETting it, caching the result in tools storage before returning
// to the caller.
func (h *toolsDownloadHandler) fetchAndCacheTools(v version.Binary, stor binarystorage.Storage, st *state.State) (io.ReadCloser, error) {
	newEnviron := stateenvirons.GetNewEnvironFunc(environs.New)
	env, err := newEnviron(st)
	if err != nil {
		return nil, err
	}
	tools, err := envtools.FindExactTools(env, v.Number, v.Series, v.Arch)
	if err != nil {
		return nil, err
	}

	// No need to verify the server's identity because we verify the SHA-256 hash.
	logger.Infof("fetching %v tools from %v", v, tools.URL)
	resp, err := utils.GetNonValidatingHTTPClient().Get(tools.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("bad HTTP response: %v", resp.Status)
		if body, err := ioutil.ReadAll(resp.Body); err == nil {
			msg += fmt.Sprintf(" (%s)", bytes.TrimSpace(body))
		}
		return nil, errors.New(msg)
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

	// Cache tarball in tools storage before returning.
	metadata := binarystorage.Metadata{
		Version: v.String(),
		Size:    tools.Size,
		SHA256:  tools.SHA256,
	}
	if err := stor.Add(bytes.NewReader(data), metadata); err != nil {
		return nil, errors.Annotate(err, "error caching tools")
	}
	return ioutil.NopCloser(bytes.NewReader(data)), nil
}

// sendTools streams the tools tarball to the client.
func (h *toolsDownloadHandler) sendTools(w http.ResponseWriter, statusCode int, tarball []byte) error {
	w.Header().Set("Content-Type", "application/x-tar-gz")
	w.Header().Set("Content-Length", fmt.Sprint(len(tarball)))
	w.WriteHeader(statusCode)
	if _, err := w.Write(tarball); err != nil {
		return errors.Trace(sendError(
			w,
			errors.NewBadRequest(errors.Annotatef(err, "failed to write tools"), ""),
		))
	}
	return nil
}

// processPost handles a tools upload POST request after authentication.
func (h *toolsUploadHandler) processPost(r *http.Request, st *state.State) (*tools.Tools, error) {
	query := r.URL.Query()

	binaryVersionParam := query.Get("binaryVersion")
	if binaryVersionParam == "" {
		return nil, errors.BadRequestf("expected binaryVersion argument")
	}
	toolsVersion, err := version.ParseBinary(binaryVersionParam)
	if err != nil {
		return nil, errors.NewBadRequest(err, fmt.Sprintf("invalid tools version %q", binaryVersionParam))
	}

	// Make sure the content type is x-tar-gz.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/x-tar-gz" {
		return nil, errors.BadRequestf("expected Content-Type: application/x-tar-gz, got: %v", contentType)
	}

	// Get the server root, so we know how to form the URL in the Tools returned.
	serverRoot, err := h.getServerRoot(r, query, st)
	if err != nil {
		return nil, errors.NewBadRequest(err, "cannot to determine server root")
	}

	// We'll clone the tools for each additional series specified.
	var cloneSeries []string
	if seriesParam := query.Get("series"); seriesParam != "" {
		cloneSeries = strings.Split(seriesParam, ",")
	}
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
	return h.handleUpload(r.Body, toolsVersions, serverRoot, st)
}

func (h *toolsUploadHandler) getServerRoot(r *http.Request, query url.Values, st *state.State) (string, error) {
	uuid := query.Get(":modeluuid")
	if uuid == "" {
		env, err := st.Model()
		if err != nil {
			return "", err
		}
		uuid = env.UUID()
	}
	return fmt.Sprintf("https://%s/model/%s", r.Host, uuid), nil
}

// handleUpload uploads the tools data from the reader to env storage as the specified version.
func (h *toolsUploadHandler) handleUpload(r io.Reader, toolsVersions []version.Binary, serverRoot string, st *state.State) (*tools.Tools, error) {
	// Check if changes are allowed and the command may proceed.
	blockChecker := common.NewBlockChecker(st)
	if err := blockChecker.ChangeAllowed(); err != nil {
		return nil, errors.Trace(err)
	}
	storage, err := st.ToolsStorage()
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
		return nil, errors.BadRequestf("no tools uploaded")
	}

	// TODO(wallyworld): check integrity of tools tarball.

	// Store tools and metadata in tools storage.
	for _, v := range toolsVersions {
		metadata := binarystorage.Metadata{
			Version: v.String(),
			Size:    int64(len(data)),
			SHA256:  sha256,
		}
		logger.Debugf("uploading tools %+v to storage", metadata)
		if err := storage.Add(bytes.NewReader(data), metadata); err != nil {
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
