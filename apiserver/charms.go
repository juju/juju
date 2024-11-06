// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/downloader"
	"github.com/juju/juju/internal/charm/services"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// charmsHTTPHandler creates is a http.Handler which serves POST
// requests to a PostHandler and GET requests to a GetHandler.
//
// TODO(katco): This is the beginning of inverting the dependencies in
// this callstack by splitting out the serving mechanism from the
// modules that are processing the requests. The next step is to
// publically expose construction of a suitable PostHandler and
// GetHandler whose goals should be clearly called out in their names,
// (e.g. charmPersitAPI for POSTs).
//
// To accomplish this, we'll have to make the httpContext type public
// so that we can pass it into these public functions.
//
// After we do this, we can then test the individual funcs/structs
// without standing up an entire HTTP server. I.e. actual unit
// tests. If you're in this area and can, please chisel away at this
// problem and update this TODO as needed! Many thanks, hacker!
//
// TODO(stickupkid): This handler is terrible, we could implement a middleware
// pattern to handle discreet logic and then pass it on to the next in the
// pipeline.
//
// As usual big methods lead to untestable code and it causes testing pain.
//
// TODO(jack-w-shaw): This handler is only used by juju clients/controllers
// 3.3 and before for both local charm upload and model migration. When we
// no longer support model migrations from 3.3 we should drop this handler
type charmsHTTPHandler struct {
	getHandler endpointMethodHandlerFunc
}

func (h *charmsHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "GET":
		err = errors.Annotate(h.getHandler(w, r), "cannot retrieve charm")
	default:
		err = emitUnsupportedMethodErr(r.Method)
	}

	if err != nil {
		if err := sendJSONError(w, r, errors.Trace(err)); err != nil {
			logger.Errorf(r.Context(), "%v", errors.Annotate(err, "cannot return error to user"))
		}
	}
}

// charmsHandler handles charm upload through HTTPS in the API server.
type charmsHandler struct {
	ctxt              httpContext
	dataDir           string
	stateAuthFunc     func(*http.Request) (*state.PooledState, error)
	objectStoreGetter ObjectStoreGetter
	logger            corelogger.Logger
}

// archiveContentSenderFunc functions are responsible for sending a
// response related to a charm archive.
type archiveContentSenderFunc func(w http.ResponseWriter, r *http.Request, archive *charm.CharmArchive) error

func (h *charmsHandler) ServeGet(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "GET" {
		return errors.Trace(emitUnsupportedMethodErr(r.Method))
	}

	if h.logger.IsLevelEnabled(corelogger.TRACE) {
		h.logger.Tracef(r.Context(), "ServeGet(%s)", r.URL)
	}

	st, _, err := h.ctxt.stateForRequestAuthenticated(r)
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Release()

	// Retrieve or list charm files.
	// Requires "url" (charm URL) and an optional "file" (the path to the
	// charm file) to be included in the query. Optionally also receives an
	// "icon" query for returning the charm icon or a default one in case the
	// charm has no icon.
	charmArchivePath, fileArg, err := h.processGet(r, st.State)
	if err != nil {
		// An error occurred retrieving the charm archive.
		if errors.Is(err, errors.NotFound) || errors.Is(err, errors.NotYetAvailable) {
			return errors.Trace(err)
		}

		return errors.NewBadRequest(err, "")
	}
	defer os.Remove(charmArchivePath)

	var sender archiveContentSenderFunc
	switch fileArg {
	case "":
		// The client requested the list of charm files.
		sender = h.manifestSender
	case "*":
		// The client requested the archive.
		sender = h.archiveSender
	default:
		// The client requested a specific file.
		sender = h.archiveEntrySender(fileArg)
	}

	return errors.Trace(sendArchiveContent(w, r, charmArchivePath, sender))
}

// manifestSender sends a JSON-encoded response to the client including the
// list of files contained in the charm archive.
func (h *charmsHandler) manifestSender(w http.ResponseWriter, r *http.Request, archive *charm.CharmArchive) error {
	manifest, err := archive.ArchiveMembers()
	if err != nil {
		return errors.Annotatef(err, "unable to read manifest in %q", archive.Path)
	}
	return errors.Trace(sendStatusAndJSON(w, http.StatusOK, &params.CharmsResponse{
		Files: manifest.SortedValues(),
	}))
}

// archiveEntrySender returns a archiveContentSenderFunc which is responsible
// for sending the contents of filePath included in the given charm archive. If
// filePath does not identify a file or a symlink, a 403 forbidden error is
// returned. If serveIcon is true, then the charm icon.svg file is sent, or a
// default icon if that file is not included in the charm.
func (h *charmsHandler) archiveEntrySender(filePath string) archiveContentSenderFunc {
	return func(w http.ResponseWriter, r *http.Request, archive *charm.CharmArchive) error {
		contents, err := common.CharmArchiveEntry(archive.Path, filePath)
		if err != nil {
			return errors.Trace(err)
		}
		ctype := mime.TypeByExtension(filepath.Ext(filePath))
		if ctype != "" {
			// Older mime.types may map .js to x-javascript.
			// Map it to javascript for consistency.
			if ctype == params.ContentTypeXJS {
				ctype = params.ContentTypeJS
			}
			w.Header().Set("Content-Type", ctype)
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(contents)))
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, bytes.NewReader(contents))
		return nil
	}
}

// archiveSender is a archiveContentSenderFunc which is responsible for sending
// the contents of the given charm archive.
func (h *charmsHandler) archiveSender(w http.ResponseWriter, r *http.Request, archive *charm.CharmArchive) error {
	// Note that http.ServeFile's error responses are not our standard JSON
	// responses (they are the usual textual error messages as produced
	// by http.Error), but there's not a great deal we can do about that,
	// except accept non-JSON error responses in the client, because
	// http.ServeFile does not provide a way of customizing its
	// error responses.
	http.ServeFile(w, r, archive.Path)
	return nil
}

// CharmUploader is an interface that is used to update the charm in
// state and upload it to the object store.
type CharmUploader interface {
	UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error)
	PrepareCharmUpload(curl string) (services.UploadedCharm, error)
	ModelUUID() string
}

// RepackageAndUploadCharm expands the given charm archive to a
// temporary directory, repackages it with the given curl's revision,
// then uploads it to storage, and finally updates the state.
func RepackageAndUploadCharm(
	ctx context.Context,
	objectStore services.Storage,
	uploader CharmUploader,
	archive *charm.CharmArchive,
	curl string,
	charmRevision int,
) (charm.Charm, string, string, string, error) {
	// Create a temp dir to contain the extracted charm dir.
	tempDir, err := os.MkdirTemp("", "charm-download")
	if err != nil {
		return nil, "", "", "", errors.Annotate(err, "cannot create temp directory")
	}
	defer os.RemoveAll(tempDir)
	extractPath := filepath.Join(tempDir, "extracted")

	// Expand and repack it with the specified revision
	archive.SetRevision(charmRevision)
	if err := archive.ExpandTo(extractPath); err != nil {
		return nil, "", "", "", errors.Annotate(err, "cannot extract uploaded charm")
	}

	charmDir, err := charm.ReadCharmDir(extractPath)
	if err != nil {
		return nil, "", "", "", errors.Annotate(err, "cannot read extracted charm")
	}

	// Try to get the version details here.
	// read just the first line of the file.
	var version string
	versionPath := filepath.Join(extractPath, "version")
	if file, err := os.Open(versionPath); err == nil {
		version, err = charm.ReadVersion(file)
		_ = file.Close()
		if err != nil {
			return nil, "", "", "", errors.Trace(err)
		}
	} else if !os.IsNotExist(err) {
		return nil, "", "", "", errors.Annotate(err, "cannot open version file")
	}

	// Bundle the charm and calculate its sha256 hash at the same time.
	var repackagedArchive bytes.Buffer
	hash := sha256.New()
	err = charmDir.ArchiveTo(io.MultiWriter(hash, &repackagedArchive))
	if err != nil {
		return nil, "", "", "", errors.Annotate(err, "cannot repackage uploaded charm")
	}
	archiveSHA256 := hex.EncodeToString(hash.Sum(nil))

	// Now we need to repackage it with the reserved URL, upload it to
	// provider storage and update the state.
	charmStorage := services.NewCharmStorage(services.CharmStorageConfig{
		Logger:       logger,
		StateBackend: uploader,
		ObjectStore:  objectStore,
	})

	storagePath, err := charmStorage.Store(ctx, curl, downloader.DownloadedCharm{
		Charm:        archive,
		CharmData:    &repackagedArchive,
		CharmVersion: version,
		Size:         int64(repackagedArchive.Len()),
		SHA256:       archiveSHA256,
		LXDProfile:   charmDir.LXDProfile(),
	})

	if err != nil {
		return nil, "", "", "", errors.Annotate(err, "cannot store charm")
	}

	return archive, archiveSHA256, version, storagePath, nil
}

type storageStateShim struct {
	*state.State
}

func (s storageStateShim) UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error) {
	ch, err := s.State.UpdateUploadedCharm(info)
	return ch, err
}

func (s storageStateShim) PrepareCharmUpload(curl string) (services.UploadedCharm, error) {
	ch, err := s.State.PrepareCharmUpload(curl)
	return ch, err
}

// processGet handles a charm file GET request after authentication.
// It returns the archive path, the requested file path (if any), whether the
// default charm icon has been requested and an error.
func (h *charmsHandler) processGet(r *http.Request, st *state.State) (
	archivePath string,
	fileArg string,
	err error,
) {
	errRet := func(err error) (string, string, error) {
		return "", "", err
	}

	query := r.URL.Query()

	// Retrieve and validate query parameters.
	curl := query.Get("url")
	if curl == "" {
		return errRet(errors.Errorf("expected url=CharmURL query argument"))
	}
	fileArg = query.Get("file")
	if fileArg != "" {
		fileArg = path.Clean(fileArg)
	}

	// Use the storage to retrieve and save the charm archive.
	ch, err := st.Charm(curl)
	if err != nil {
		return errRet(errors.Annotate(err, "cannot get charm from state"))
	}
	// Check if the charm is still pending to be downloaded and return back
	// a suitable error.
	if !ch.IsUploaded() {
		return errRet(errors.NewNotYetAvailable(nil, curl))
	}

	// Get the underlying object store for the model UUID, which we can then
	// retrieve the blob from.
	store, err := h.objectStoreGetter.GetObjectStore(r.Context(), st.ModelUUID())
	if err != nil {
		return errRet(errors.Annotate(err, "cannot get object store"))
	}

	archivePath, err = common.ReadCharmFromStorage(r.Context(), store, h.dataDir, ch.StoragePath())
	if err != nil {
		return errRet(errors.Annotatef(err, "cannot read charm %q from storage", curl))
	}
	return archivePath, fileArg, nil
}

// sendJSONError sends a JSON-encoded error response.  Note the
// difference from the error response sent by the sendError function -
// the error is encoded in the Error field as a string, not an Error
// object.
func sendJSONError(w http.ResponseWriter, req *http.Request, err error) error {
	if errors.Is(err, errors.NotYetAvailable) {
		// This error is typically raised when trying to fetch the blob
		// contents for a charm which is still pending to be downloaded.
		//
		// We should log this at debug level to avoid unnecessary noise
		// in the logs.
		logger.Debugf(req.Context(), "returning error from %s %s: %s", req.Method, req.URL, errors.Details(err))
	} else {
		logger.Errorf(req.Context(), "returning error from %s %s: %s", req.Method, req.URL, errors.Details(err))
	}

	perr, status := apiservererrors.ServerErrorAndStatus(err)
	return errors.Trace(sendStatusAndJSON(w, status, &params.CharmsResponse{
		Error:     perr.Message,
		ErrorCode: perr.Code,
		ErrorInfo: perr.Info,
	}))
}

// sendArchiveContent uses the given archiveContentSenderFunc to send a
// response related to the charm archive located in the given
// archivePath.
func sendArchiveContent(
	w http.ResponseWriter,
	r *http.Request,
	archivePath string,
	sender archiveContentSenderFunc,
) error {
	logger.Child("charmhttp").Tracef(r.Context(), "sendArchiveContent %q", archivePath)
	archive, err := charm.ReadCharmArchive(archivePath)
	if err != nil {
		return errors.Annotatef(err, "unable to read archive in %q", archivePath)
	}
	// The archiveContentSenderFunc will set up and send an appropriate response.
	if err := sender(w, r, archive); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func writeCharmToTempFile(r io.Reader) (string, error) {
	tempFile, err := os.CreateTemp("", "charm")
	if err != nil {
		return "", errors.Annotate(err, "creating temp file")
	}
	defer tempFile.Close()

	if _, err := io.Copy(tempFile, r); err != nil {
		return "", errors.Annotate(err, "processing upload")
	}
	return tempFile.Name(), nil
}

func modelIsImporting(st *state.State) (bool, error) {
	model, err := st.Model()
	if err != nil {
		return false, errors.Trace(err)
	}
	return model.MigrationMode() == state.MigrationModeImporting, nil
}

func emitUnsupportedMethodErr(method string) error {
	return errors.MethodNotAllowedf("unsupported method: %q", method)
}
