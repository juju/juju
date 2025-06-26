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
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	agentbinaryerrors "github.com/juju/juju/domain/agentbinary/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	internalerrors "github.com/juju/juju/internal/errors"
	jujuhttp "github.com/juju/juju/internal/http"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/state/stateenvirons"
)

// AgentBinaryStore is an interface that provides the ability to store new agent
// binaries into a store for the controller or model.
type AgentBinaryStore interface {
	// AddAgentBinaryWithSHA256 adds a new agent binary to the object store and
	// saves its metadata to the database.
	// The following errors can be returned:
	// - [coreerrors.NotSupported] if the architecture is not supported.
	// - [agentbinaryerrors.AlreadyExists] if an agent binary already exists for
	// this version and architecture.
	// - [agentbinaryerrors.ObjectNotFound] if there was a problem referencing
	// the agent binary metadata with the previously saved binary object.
	// This error should be considered an internal problem. It is discussed here
	// to make the caller aware of future problems.
	// - [coreerrors.NotValid] if the agent version is not valid.
	// - [agentbinaryerrors.HashMismatch] when the expected sha does not match
	// that which was computed against the binary data.
	AddAgentBinaryWithSHA256(
		ctx context.Context,
		data io.Reader,
		version coreagentbinary.Version,
		dataSize int64,
		dataSHA256Sum string,
	) error
}

// agentBinaryStoreLogStore is a wrapper around the [AgentBinaryStore] that
// intercepts add binary requests and logs the fact that a binary is being added
// with context about the caller of the add operation.
type agentBinaryStoreLogShim struct {
	// AgentBinaryStore is the [AgentBinaryStore] to wrap.
	AgentBinaryStore

	// StoreName represents a canonical name to assocaite to the store being
	// wrapped. This is used in the subsequent log message to identify the
	// destination of the add.
	StoreName string
}

// AgentBinaryStoreGetter is a deferred type that can be used to get an
// AgentBinaryStore at exactly the time it is needed. This allows for
// context aware answers to be made.
type AgentBinaryStoreGetter func(*http.Request) (AgentBinaryStore, error)

// BlockChecker checks for current blocks if any.
type BlockChecker interface {
	// ChangeAllowed checks if change block is in place.
	// Change block prevents all operations that may change
	// current model in any way from running successfully.
	ChangeAllowed(context.Context) error
}

// DomainServicesGetter describes a type that can be used for getting
// [services.DomainServices] from a given context that comes from a http
// request.
type DomainServicesGetter func(ctx context.Context) (services.DomainServices, error)

// AddAgentBinaryWithSHA256 is a wrapper around
// [AgentBinaryStore.AddAgentBinaryWithSHA256] that logs out the fact an agent
// binary is being added to the store identified by
// [agentBinaryStoreLogShim.StoreName]. As part of the log message an entity is
// established that initiated this call helping identifying the who behind the
// operation.
func (a *agentBinaryStoreLogShim) AddAgentBinaryWithSHA256(
	ctx context.Context,
	data io.Reader,
	version coreagentbinary.Version,
	dataSize int64,
	dataSHA256Sum string,
) error {
	logger.Infof(
		ctx,
		"agent binaries being added to %q for %q with sha %q on behalf of entity %q",
		a.StoreName, version.String(), dataSHA256Sum, httpcontext.EntityForContext(ctx),
	)
	return a.AgentBinaryStore.AddAgentBinaryWithSHA256(ctx, data, version, dataSize, dataSHA256Sum)
}

// BlockCheckerGetterForServices returns a [BlockCheckerGetter] that is
// constructed from the supplied context.
func BlockCheckerGetterForServices(servicesGetter DomainServicesGetter) func(context.Context) (BlockChecker, error) {
	return func(ctx context.Context) (BlockChecker, error) {
		svc, err := servicesGetter(ctx)
		if err != nil {
			return nil, err
		}

		return common.NewBlockChecker(svc.BlockCommand()), nil
	}
}

// controllerAgentBinaryStoreForHTTPContext provides a deferred getter that
// will provide the controller's [AgentBinaryStore] for the given [httpContext].
func controllerAgentBinaryStoreForHTTPContext(httpCtx httpContext) AgentBinaryStoreGetter {
	return func(r *http.Request) (AgentBinaryStore, error) {
		services, err := httpCtx.domainServicesForRequest(r.Context())
		if err != nil {
			return nil, internalerrors.Capture(err)
		}

		return &agentBinaryStoreLogShim{
			AgentBinaryStore: services.ControllerAgentBinaryStore(),
			StoreName:        "controller agent binary store",
		}, nil
	}
}

// migratingAgentBinaryStoreForHTTPContext provides a deferred getter that will
// provide the agent binary store for the model that is being migrated as part
// of the request.
func migratingAgentBinaryStoreForHTTPContext(httpCtx httpContext) AgentBinaryStoreGetter {
	return func(r *http.Request) (AgentBinaryStore, error) {
		services, err := httpCtx.domainServicesDuringMigrationForRequest(r)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}

		modelUUID, exists := httpcontext.MigrationRequestModelUUID(r)
		if !exists {
			modelUUID = "unknown"
		}

		return &agentBinaryStoreLogShim{
			AgentBinaryStore: services.AgentBinaryStore(),
			StoreName:        "model " + modelUUID,
		}, nil
	}
}

// modelAgentBinaryStoreForHTTPContext provides a deferred getter that will
// provide the models [AgentBinaryStore] for the given [httpContext].
func modelAgentBinaryStoreForHTTPContext(httpCtx httpContext) AgentBinaryStoreGetter {
	return func(r *http.Request) (AgentBinaryStore, error) {
		services, err := httpCtx.domainServicesForRequest(r.Context())
		if err != nil {
			return nil, internalerrors.Capture(err)
		}

		modelUUID, exists := httpcontext.RequestModelUUID(r.Context())
		if !exists {
			return nil, internalerrors.New(
				"getting agent binary store for model, request does not contain model information",
			)
		}

		return &agentBinaryStoreLogShim{
			AgentBinaryStore: services.AgentBinaryStore(),
			StoreName:        "model " + modelUUID,
		}, nil
	}
}

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

// toolsHandler handles agent binary uploads through HTTPS in the API server. We
// still refer to the handler with the word tools as the apiserver paths that
// this is exposed through still encompasses this wording.
type toolsUploadHandler struct {
	blockCheckerGetter func(context.Context) (BlockChecker, error)
	storeGetter        AgentBinaryStoreGetter
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

// newToolsUploadHandler constructs a new [toolsUploadHandler] from the supplied
// arguments.
func newToolsUploadHandler(
	blockChecker func(context.Context) (BlockChecker, error),
	storeGetter AgentBinaryStoreGetter,
) *toolsUploadHandler {
	return &toolsUploadHandler{
		blockCheckerGetter: blockChecker,
		storeGetter:        storeGetter,
	}
}

func (h *toolsDownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	st, err := h.ctxt.stateForRequestUnauthenticated(r.Context())
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf(r.Context(), "%v", err)
		}
		return
	}
	defer st.Release()

	switch r.Method {
	case "GET":
		reader, size, err := h.getToolsForRequest(r, st.State)
		if err != nil {
			logger.Errorf(r.Context(), "GET(%s) failed: %v", r.URL, err)
			if err := sendError(w, errors.NewBadRequest(err, "")); err != nil {
				logger.Errorf(r.Context(), "%v", err)
			}
			return
		}
		defer reader.Close()
		if err := h.sendTools(w, reader, size); err != nil {
			logger.Errorf(r.Context(), "%v", err)
		}
	default:
		if err := sendError(w, errors.MethodNotAllowedf("unsupported method: %q", r.Method)); err != nil {
			logger.Errorf(r.Context(), "%v", err)
		}
	}
}

func (h *toolsUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		// Add tools to storage.
		uploadedTools, err := h.processPost(r)
		if err != nil {
			if err := sendError(w, err); err != nil {
				logger.Errorf(r.Context(), "sending err response for post upload tools request: %v", err)
			}
			return
		}
		if err := internalhttp.SendStatusAndJSON(w, http.StatusOK, &params.ToolsResult{
			ToolsList: tools.List{&uploadedTools},
		}); err != nil {
			logger.Errorf(r.Context(), "%v", err)
		}
	default:
		if err := sendError(w, errors.MethodNotAllowedf("unsupported method: %q", r.Method)); err != nil {
			logger.Errorf(r.Context(), "%v", err)
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
	logger.Debugf(r.Context(), "request for agent binaries: %s", vers)

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
		logger.Infof(r.Context(), "%v agent binaries not found locally, fetching", vers)
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
	if err != nil && !errors.Is(err, agentbinaryerrors.AlreadyExists) {
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
	domainServices, err := h.ctxt.domainServicesForRequest(ctx)
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
	logger.Infof(ctx, "fetching %v agent binaries from %v", v, exactTools.URL)
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

// processPost handles a tools upload POST request after authentication. It
// checks that the binary version supplied is valid and that the uploader has
// set the right content type before handling the uploaded data.
func (h *toolsUploadHandler) processPost(r *http.Request) (tools.Tools, error) {
	query := r.URL.Query()
	binaryVersionParam := query.Get("binaryVersion")
	if binaryVersionParam == "" {
		return tools.Tools{}, internalerrors.New(
			"expected binaryVersion argument",
		).Add(coreerrors.BadRequest)
	}

	parsedBinaryVersion, err := semversion.ParseBinary(binaryVersionParam)
	if err != nil {
		return tools.Tools{}, internalerrors.Errorf(
			"invalid agent binary version %q",
			binaryVersionParam,
		).Add(coreerrors.BadRequest)
	}

	agentBinaryVersion := coreagentbinary.Version{
		Number: parsedBinaryVersion.Number,
		Arch:   parsedBinaryVersion.Arch,
	}

	// Make sure the content type is x-tar-gz.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/x-tar-gz" {
		return tools.Tools{}, internalerrors.Errorf(
			"expected Content-Type: application/x-tar-gz, got: %v", contentType,
		).Add(coreerrors.BadRequest)
	}

	agentBinaryStore, err := h.storeGetter(r)
	if err != nil {
		return tools.Tools{}, internalerrors.Errorf(
			"getting agent binary store for tools upload request: %w", err,
		)
	}

	size, sha, err := h.handleUpload(r.Context(), r.Body, agentBinaryStore, agentBinaryVersion)
	if err != nil {
		return tools.Tools{}, internalerrors.Capture(err)
	}

	serverRoot, err := h.getServerRoot(r, query)
	if err != nil {
		return tools.Tools{}, internalerrors.Capture(err)
	}

	return tools.Tools{
		Version: parsedBinaryVersion,
		URL:     common.ToolsURL(serverRoot, parsedBinaryVersion.String()),
		SHA256:  sha,
		Size:    size,
	}, nil
}

func (h *toolsUploadHandler) getServerRoot(r *http.Request, query url.Values) (string, error) {
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
	agentBinaryStore AgentBinaryStore,
	agentBinaryVersion coreagentbinary.Version,
) (int64, string, error) {
	blockChecker, err := h.blockCheckerGetter(ctx)
	if err != nil {
		return 0, "", internalerrors.Errorf(
			"failed getting block checker for tools upload request: %w", err,
		)
	}

	if err := blockChecker.ChangeAllowed(ctx); err != nil {
		return 0, "", internalerrors.Capture(err)
	}

	// Read the tools tarball from the request, calculating the sha256 along the way.
	data, sha256, size, err := tmpCacheAndHash(r)
	if err != nil {
		return 0, "", internalerrors.Errorf("caching and hashing agent binary upload: %w", err)
	}
	defer data.Close()

	if size == 0 {
		return 0, "", internalerrors.New("no agent binaries uploaded").Add(coreerrors.BadRequest)
	}

	// TODO(wallyworld, tlm): check integrity of tools tarball. This todo was
	// added before the integration of Dqlite into this handler. What we ideally
	// should be doing is letting the agent binary store disect the upload if we
	// wish for this to be done.

	logger.Debugf(
		ctx,
		"uploading agent binaries for version %q and arch %q to agent binary store",
		agentBinaryVersion.Number.String(), agentBinaryVersion.Arch,
	)

	err = agentBinaryStore.AddAgentBinaryWithSHA256(
		ctx, data, agentBinaryVersion, size, sha256,
	)
	switch {
	// Happens when the agent binary version isn't valid.
	case errors.Is(err, coreerrors.NotValid):
		err = internalerrors.Errorf(
			"agent binary version %q is not valid", agentBinaryVersion,
		).Add(coreerrors.BadRequest)
	// Happens when the agent binary version architecture isn't supported.
	case errors.Is(err, coreerrors.NotSupported):
		err = internalerrors.Errorf(
			"unsupported architecture %q", agentBinaryVersion.Arch,
		).Add(coreerrors.BadRequest)
	// Happens when the agent binary version being uploaded for already exists.
	// We never want to allow someone to overwrite an established agent binary
	// for a version by overwriting it with new data.
	case errors.Is(err, agentbinaryerrors.AlreadyExists):
		err = internalerrors.Errorf(
			"agent binary already exists for version %q and arch %q",
			agentBinaryVersion.Number, agentBinaryVersion.Arch,
		).Add(coreerrors.BadRequest)
	// Unknown error. This case is considered an internal server error unrelated
	// to any bad or missing infomration in the upload request.
	case err != nil:
		err = internalerrors.Errorf(
			"unable to add uploaded agent binary for version %q and arch %q: %w",
			agentBinaryVersion.Number, agentBinaryVersion.Arch, err,
		)
	}

	return size, sha256, err
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
