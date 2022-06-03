// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/im7mortal/kmutex"
	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/version/v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/httpcontext"
	coreos "github.com/juju/juju/core/os"
	coreseries "github.com/juju/juju/core/series"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/tools"
)

// toolsReadCloser wraps the ReadCloser for the tools blob
// and the state StorageCloser.
// It allows us to stream the tools binary from state,
// closing them both at once when done.
type toolsReadCloser struct {
	f  io.ReadCloser
	st binarystorage.StorageCloser
}

func (t *toolsReadCloser) Read(p []byte) (n int, err error) {
	return t.f.Read(p)
}

func (t *toolsReadCloser) Close() error {
	var err error
	if err = t.f.Close(); err == nil {
		return t.st.Close()
	}
	if err2 := t.st.Close(); err2 != nil {
		err = errors.Wrap(err, err2)
	}
	return err
}

// toolsHandler handles tool upload through HTTPS in the API server.
type toolsUploadHandler struct {
	ctxt          httpContext
	stateAuthFunc func(*http.Request) (*state.PooledState, error)
}

// toolsHandler handles tool download through HTTPS in the API server.
type toolsDownloadHandler struct {
	ctxt       httpContext
	fetchMutex *kmutex.Kmutex
}

func newToolsDownloadHandler(httpCtxt httpContext) *toolsDownloadHandler {
	return &toolsDownloadHandler{
		ctxt:       httpCtxt,
		fetchMutex: kmutex.New(),
	}
}

func (h *toolsDownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	st, err := h.ctxt.stateForRequestUnauthenticated(r)
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	defer st.Release()

	switch r.Method {
	case "GET":
		reader, size, err := h.getToolsForRequest(r, st.State)
		if err != nil {
			logger.Errorf("GET(%s) failed: %v", r.URL, err)
			if err := sendError(w, errors.NewBadRequest(err, "")); err != nil {
				logger.Errorf("%v", err)
			}
			return
		}
		defer reader.Close()
		if err := h.sendTools(w, reader, size); err != nil {
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
	st, err := h.stateAuthFunc(r)
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	defer st.Release()

	switch r.Method {
	case "POST":
		// Add tools to storage.
		agentTools, err := h.processPost(r, st.State)
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

// getToolsForRequest retrieves the compressed agent binaries tarball from state
// based on the input HTTP request.
// It is returned with the size of the file as recorded in the stored metadata.
func (h *toolsDownloadHandler) getToolsForRequest(r *http.Request, st *state.State) (_ io.ReadCloser, _ int64, err error) {
	vers, err := version.ParseBinary(r.URL.Query().Get(":version"))
	if err != nil {
		return nil, 0, errors.Annotate(err, "error parsing version")
	}
	logger.Debugf("request for agent binaries: %s", vers)

	storage, err := st.ToolsStorage()
	if err != nil {
		return nil, 0, errors.Annotate(err, "error getting storage for agent binaries")
	}
	defer func() {
		if err != nil {
			_ = storage.Close()
		}
	}()

	// TODO(juju4) = remove this compatibility logic
	// Looked for stored tools which are recorded for a series
	// but which have the same os type as the wanted version.
	// Alternatively, the request may have been for a specifc
	// series and we need to use stored tools for the corresponding
	// os type.
	storageVers := vers
	var osTypeName string
	if vers.Number.Major == 2 && vers.Number.Minor <= 8 {
		wantedOSType := vers.Release
		if !coreos.IsValidOSTypeName(vers.Release) {
			wantedOSType = coreseries.DefaultOSTypeNameFromSeries(vers.Release)
		}
		vers.Release = wantedOSType

		all, err := storage.AllMetadata()
		if err != nil {
			return nil, 0, errors.Trace(err)
		}
		var osMatchVersion *version.Binary
		for _, m := range all {
			metaVers, err := version.ParseBinary(m.Version)
			if err != nil {
				return nil, 0, errors.Annotate(err, "error parsing metadata version")
			}

			// Exact match so just use that with os type name substitution.
			if m.Version == vers.String() {
				osMatchVersion = &metaVers
				break
			}
			if osMatchVersion != nil {
				continue
			}
			metaOSType := metaVers.Release
			if !coreos.IsValidOSTypeName(metaVers.Release) {
				metaOSType = coreseries.DefaultOSTypeNameFromSeries(metaVers.Release)
			}
			toCompare := metaVers
			toCompare.Release = strings.ToLower(metaOSType)
			if toCompare.String() == vers.String() {
				logger.Debugf("using os based version %s for requested %s", toCompare, vers)
				osMatchVersion = &metaVers
				osTypeName = toCompare.Release
			}
		}
		// Set the version to store to be the match we found
		// for any compatible series.
		if osMatchVersion != nil {
			storageVers = *osMatchVersion
		}
	}

	locker := h.fetchMutex.Locker(storageVers.String())
	locker.Lock()
	defer locker.Unlock()

	md, reader, err := storage.Open(storageVers.String())
	if errors.IsNotFound(err) {
		// Tools could not be found in tools storage,
		// so look for them in simplestreams,
		// fetch them and cache in tools storage.
		logger.Infof("%v agent binaries not found locally, fetching", vers)
		if osTypeName != "" {
			storageVers.Release = osTypeName
		}
		err = h.fetchAndCacheTools(vers, storageVers, st, storage)
		if err != nil {
			err = errors.Annotate(err, "error fetching agent binaries")
		} else {
			md, reader, err = storage.Open(storageVers.String())
		}
	}
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	return &toolsReadCloser{f: reader, st: storage}, md.Size, nil
}

// fetchAndCacheTools fetches tools with the specified version by searching for a URL
// in simplestreams and GETting it, caching the result in tools storage before returning
// to the caller.
func (h *toolsDownloadHandler) fetchAndCacheTools(
	v version.Binary,
	storageVers version.Binary,
	st *state.State,
	modelStorage binarystorage.Storage,
) error {
	systemState, err := h.ctxt.statePool().SystemState()
	if err != nil {
		return errors.Trace(err)
	}

	controllerModel, err := systemState.Model()
	if err != nil {
		return err
	}

	var model *state.Model
	var storage binarystorage.Storage
	switch controllerModel.Type() {
	case state.ModelTypeCAAS:
		// TODO(caas): unify tool fetching
		// Cache the tools against the model when the controller is CAAS.
		model, err = st.Model()
		if err != nil {
			return err
		}
		storage = modelStorage
	case state.ModelTypeIAAS:
		// Cache the tools against the controller when the controller is IAAS.
		model = controllerModel
		controllerStorage, err := systemState.ToolsStorage()
		if err != nil {
			return err
		}
		defer controllerStorage.Close()
		storage = controllerStorage
	default:
		return errors.NotValidf("model type %q", controllerModel.Type())
	}

	newEnviron := stateenvirons.GetNewEnvironFunc(environs.New)
	env, err := newEnviron(model)
	if err != nil {
		return err
	}

	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	exactTools, err := envtools.FindExactTools(ss, env, v.Number, v.Release, v.Arch)
	if err != nil {
		return err
	}

	// No need to verify the server's identity because we verify the SHA-256 hash.
	logger.Infof("fetching %v agent binaries from %v", v, exactTools.URL)
	client := jujuhttp.NewClient(jujuhttp.WithSkipHostnameVerification(true))
	resp, err := client.Get(context.TODO(), exactTools.URL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("bad HTTP response: %v", resp.Status)
		if body, err := ioutil.ReadAll(resp.Body); err == nil {
			msg += fmt.Sprintf(" (%s)", bytes.TrimSpace(body))
		}
		return errors.New(msg)
	}

	data, respSha256, size, err := tmpCacheAndHash(resp.Body)
	if err != nil {
		return err
	}
	defer data.Close()
	if size != exactTools.Size {
		return errors.Errorf("size mismatch for %s", exactTools.URL)
	}
	if respSha256 != exactTools.SHA256 {
		return errors.Errorf("hash mismatch for %s", exactTools.URL)
	}

	md := binarystorage.Metadata{
		Version: storageVers.String(),
		Size:    exactTools.Size,
		SHA256:  exactTools.SHA256,
	}
	if err := storage.Add(data, md); err != nil {
		return errors.Annotate(err, "error caching agent binaries")
	}

	return nil
}

// sendTools streams the tools tarball to the client.
func (h *toolsDownloadHandler) sendTools(w http.ResponseWriter, reader io.ReadCloser, size int64) error {
	logger.Tracef("sending %d bytes", size)

	w.Header().Set("Content-Type", "application/x-tar-gz")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))

	if _, err := io.Copy(w, reader); err != nil {
		// Having begun writing, it is too late to send an error response here.
		return errors.Annotatef(err, "failed to send agent binaries")
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
		return nil, errors.NewBadRequest(err, fmt.Sprintf("invalid agent binaries version %q", binaryVersionParam))
	}

	// Make sure the content type is x-tar-gz.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/x-tar-gz" {
		return nil, errors.BadRequestf("expected Content-Type: application/x-tar-gz, got: %v", contentType)
	}

	logger.Debugf("request to upload agent binaries: %s", toolsVersion)
	// TODO(juju4) - drop this compatibility with series params
	// If the binary is for a workload series, convert the release to an OS type name.
	allSeries, err := coreseries.AllWorkloadSeries("", "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	if allSeries.Contains(toolsVersion.Release) {
		toolsVersion.Release = coreseries.DefaultOSTypeNameFromSeries(toolsVersion.Release)
	}

	toolsVersions := []version.Binary{toolsVersion}
	serverRoot := h.getServerRoot(r, query, st)
	return h.handleUpload(r.Body, toolsVersions, serverRoot, st)
}

func (h *toolsUploadHandler) getServerRoot(r *http.Request, query url.Values, st *state.State) string {
	modelUUID := httpcontext.RequestModelUUID(r)
	return fmt.Sprintf("https://%s/model/%s", r.Host, modelUUID)
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
	data, sha256, size, err := tmpCacheAndHash(r)
	if err != nil {
		return nil, err
	}
	defer data.Close()

	if size == 0 {
		return nil, errors.BadRequestf("no agent binaries uploaded")
	}

	// TODO(wallyworld): check integrity of tools tarball.

	// Store tools and metadata in tools storage.
	for _, v := range toolsVersions {
		metadata := binarystorage.Metadata{
			Version: v.String(),
			Size:    size,
			SHA256:  sha256,
		}
		logger.Debugf("uploading agent binaries %+v to storage", metadata)
		if err := storage.Add(data, metadata); err != nil {
			return nil, err
		}
	}

	tools := &tools.Tools{
		Version: toolsVersions[0],
		Size:    size,
		SHA256:  sha256,
		URL:     common.ToolsURL(serverRoot, toolsVersions[0]),
	}
	return tools, nil
}

type cleanupCloser struct {
	io.ReadCloser
	cleanup func()
}

func (c *cleanupCloser) Close() error {
	if c.cleanup != nil {
		c.cleanup()
	}
	return c.ReadCloser.Close()
}

func tmpCacheAndHash(r io.Reader) (data io.ReadCloser, sha256hex string, size int64, err error) {
	tmpFile, err := ioutil.TempFile("", "jujutools*")
	tmpFilename := tmpFile.Name()
	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFilename)
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()
	tr := io.TeeReader(r, tmpFile)
	hasher := sha256.New()
	_, err = io.Copy(hasher, tr)
	if err != nil {
		return nil, "", 0, errors.Annotatef(err, "failed to hash agent tools and write to file %q", tmpFilename)
	}
	_, err = tmpFile.Seek(0, 0)
	if err != nil {
		return nil, "", 0, errors.Trace(err)
	}
	stat, err := tmpFile.Stat()
	if err != nil {
		return nil, "", 0, errors.Trace(err)
	}
	return &cleanupCloser{tmpFile, cleanup}, fmt.Sprintf("%x", hasher.Sum(nil)), stat.Size(), nil
}
