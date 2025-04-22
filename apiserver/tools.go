// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/im7mortal/kmutex"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/httpcontext"
	internalhttp "github.com/juju/juju/apiserver/internal/http"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/state/stateenvirons"
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
	st, err := h.ctxt.stateForRequestUnauthenticated(r.Context())
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf(context.TODO(), "%v", err)
		}
		return
	}
	defer st.Release()

	switch r.Method {
	case "GET":
		reader, size, err := h.getToolsForRequest(r, st.State)
		if err != nil {
			logger.Errorf(context.TODO(), "GET(%s) failed: %v", r.URL, err)
			if err := sendError(w, errors.NewBadRequest(err, "")); err != nil {
				logger.Errorf(context.TODO(), "%v", err)
			}
			return
		}
		defer reader.Close()
		if err := h.sendTools(w, reader, size); err != nil {
			logger.Errorf(context.TODO(), "%v", err)
		}
	default:
		if err := sendError(w, errors.MethodNotAllowedf("unsupported method: %q", r.Method)); err != nil {
			logger.Errorf(context.TODO(), "%v", err)
		}
	}
}

func (h *toolsUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Validate before authenticate because the authentication is dependent
	// on the state connection that is determined during the validation.
	st, err := h.stateAuthFunc(r)
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf(context.TODO(), "%v", err)
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
				logger.Errorf(context.TODO(), "%v", err)
			}
			return
		}
		if err := internalhttp.SendStatusAndJSON(w, http.StatusOK, &params.ToolsResult{
			ToolsList: tools.List{agentTools},
		}); err != nil {
			logger.Errorf(context.TODO(), "%v", err)
		}
	default:
		if err := sendError(w, errors.MethodNotAllowedf("unsupported method: %q", r.Method)); err != nil {
			logger.Errorf(context.TODO(), "%v", err)
		}
	}
}

// getToolsForRequest retrieves the compressed agent binaries tarball from state
// based on the input HTTP request.
// It is returned with the size of the file as recorded in the stored metadata.
func (h *toolsDownloadHandler) getToolsForRequest(r *http.Request, st *state.State) (_ io.ReadCloser, _ int64, err error) {
	vers, err := semversion.ParseBinary(r.URL.Query().Get(":version"))
	if err != nil {
		return nil, 0, errors.Annotate(err, "error parsing version")
	}
	logger.Debugf(context.TODO(), "request for agent binaries: %s", vers)

	store, err := h.ctxt.controllerObjectStoreForRequest(r.Context())
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	storage, err := st.ToolsStorage(store)
	if err != nil {
		return nil, 0, errors.Annotate(err, "error getting storage for agent binaries")
	}
	defer func() {
		if err != nil {
			_ = storage.Close()
		}
	}()

	locker := h.fetchMutex.Locker(vers.String())
	locker.Lock()
	defer locker.Unlock()

	md, reader, err := storage.Open(r.Context(), vers.String())
	if errors.Is(err, errors.NotFound) {
		// Tools could not be found in tools storage,
		// so look for them in simplestreams,
		// fetch them and cache in tools storage.
		logger.Infof(context.TODO(), "%v agent binaries not found locally, fetching", vers)
		err = h.fetchAndCacheTools(r.Context(), vers, st, storage, store)
		if err != nil {
			err = errors.Annotate(err, "error fetching agent binaries")
		} else {
			md, reader, err = storage.Open(r.Context(), vers.String())
		}
	}
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	defer func() { _ = reader.Close() }()

	domainServices, err := h.ctxt.domainServicesForRequest(r.Context())
	if err != nil {
		return nil, 0, errors.Trace(err)
	}

	// TODO (tlm): This is a temporary workaround in the transition to Dqlite.
	//  This will be removed as part of JUJU-7812. We need to dual
	//  write what is downloaded to the model agent store so that migration
	//  continues to work.
	dataCache := bytes.NewBuffer(nil)
	agentStream := io.TeeReader(reader, dataCache)
	metadata, err := storage.Metadata(vers.String())
	if err != nil {
		return nil, 0, errors.Annotatef(err, "getting metadata for agent binary version %s", vers.String())
	}

	err = domainServices.AgentBinaryStore().AddAgentBinaryWithSHA256(
		r.Context(),
		agentStream,
		coreagentbinary.Version{
			Number: vers.Number,
			Arch:   vers.Arch,
		},
		md.Size,
		metadata.SHA256,
	)
	if err != nil {
		logger.Errorf(r.Context(), "replicating downloaded agent binary into model store: %v", err)
	}

	return &toolsReadCloser{f: io.NopCloser(dataCache), st: storage}, md.Size, nil
}

// fetchAndCacheTools fetches tools with the specified version by searching for a URL
// in simplestreams and GETting it, caching the result in tools storage before returning
// to the caller.
func (h *toolsDownloadHandler) fetchAndCacheTools(
	ctx context.Context,
	v semversion.Binary,
	st *state.State,
	modelStorage binarystorage.Storage,
	store objectstore.ObjectStore,
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
		controllerStorage, err := systemState.ToolsStorage(store)
		if err != nil {
			return err
		}
		defer controllerStorage.Close()
		storage = controllerStorage
	default:
		return errors.NotValidf("model type %q", controllerModel.Type())
	}

	newEnviron := stateenvirons.GetNewEnvironFunc(environs.New)
	domainServices, err := h.ctxt.srv.shared.domainServicesGetter.ServicesForModel(ctx, coremodel.UUID(st.ModelUUID()))
	if err != nil {
		return err
	}
	env, err := newEnviron(model, domainServices.Cloud(), domainServices.Credential(), domainServices.Config())
	if err != nil {
		return err
	}

	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	exactTools, err := envtools.FindExactTools(ctx, ss, env, v.Number, v.Release, v.Arch)
	if err != nil {
		return err
	}

	// No need to verify the server's identity because we verify the SHA-256 hash.
	logger.Infof(context.TODO(), "fetching %v agent binaries from %v", v, exactTools.URL)
	client := jujuhttp.NewClient(jujuhttp.WithSkipHostnameVerification(true))
	resp, err := client.Get(ctx, exactTools.URL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("bad HTTP response: %v", resp.Status)
		if body, err := io.ReadAll(resp.Body); err == nil {
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

	dataCache := bytes.NewBuffer(nil)
	reader := io.TeeReader(data, dataCache)

	md := binarystorage.Metadata{
		Version: v.String(),
		Size:    exactTools.Size,
		SHA256:  exactTools.SHA256,
	}
	if err := storage.Add(ctx, reader, md); err != nil {
		return errors.Annotate(err, "error caching agent binaries")
	}

	return nil
}

// sendTools streams the tools tarball to the client.
func (h *toolsDownloadHandler) sendTools(w http.ResponseWriter, reader io.ReadCloser, size int64) error {
	logger.Tracef(context.TODO(), "sending %d bytes", size)

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
	toolsVersion, err := semversion.ParseBinary(binaryVersionParam)
	if err != nil {
		return nil, errors.NewBadRequest(err, fmt.Sprintf("invalid agent binaries version %q", binaryVersionParam))
	}

	// Make sure the content type is x-tar-gz.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/x-tar-gz" {
		return nil, errors.BadRequestf("expected Content-Type: application/x-tar-gz, got: %v", contentType)
	}

	logger.Debugf(context.TODO(), "request to upload agent binaries: %s", toolsVersion)
	toolsVersions := []semversion.Binary{toolsVersion}
	serverRoot, err := h.getServerRoot(r, query, st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	store, err := h.ctxt.controllerObjectStoreForRequest(r.Context())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return h.handleUpload(r.Context(), r.Body, toolsVersions, serverRoot, st, store)
}

func (h *toolsUploadHandler) getServerRoot(r *http.Request, query url.Values, st *state.State) (string, error) {
	modelUUID, valid := httpcontext.RequestModelUUID(r.Context())
	if !valid {
		return "", errors.BadRequestf("invalid model UUID")
	}
	return fmt.Sprintf("https://%s/model/%s", r.Host, modelUUID), nil
}

// handleUpload uploads the tools data from the reader to env storage as the specified version.
func (h *toolsUploadHandler) handleUpload(
	ctx context.Context,
	r io.Reader,
	toolsVersions []semversion.Binary,
	serverRoot string,
	st *state.State,
	store objectstore.ObjectStore,
) (*tools.Tools, error) {
	serviceFactory, err := h.ctxt.domainServicesForRequest(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Check if changes are allowed and the command may proceed.
	blockChecker := common.NewBlockChecker(serviceFactory.BlockCommand())
	if err := blockChecker.ChangeAllowed(ctx); err != nil {
		return nil, errors.Trace(err)
	}
	storage, err := st.ToolsStorage(store)
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
		logger.Debugf(context.TODO(), "uploading agent binaries %+v to storage", metadata)
		if err := storage.Add(ctx, data, metadata); err != nil {
			return nil, err
		}
	}

	tools := &tools.Tools{
		Version: toolsVersions[0],
		Size:    size,
		SHA256:  sha256,
		URL:     common.ToolsURL(serverRoot, toolsVersions[0].String()),
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
	tmpFile, err := os.CreateTemp("", "jujutools*")
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
