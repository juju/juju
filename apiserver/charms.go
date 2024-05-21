// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/internal/charm"
	ziputil "github.com/juju/utils/v4/zip"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	corelogger "github.com/juju/juju/core/logger"
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
	postHandler endpointMethodHandlerFunc
	getHandler  endpointMethodHandlerFunc
}

func (h *charmsHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "POST":
		err = errors.Annotate(h.postHandler(w, r), "cannot upload charm")
	case "GET":
		err = errors.Annotate(h.getHandler(w, r), "cannot retrieve charm")
	default:
		err = emitUnsupportedMethodErr(r.Method)
	}

	if err != nil {
		if err := sendJSONError(w, r, errors.Trace(err)); err != nil {
			logger.Errorf("%v", errors.Annotate(err, "cannot return error to user"))
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

// bundleContentSenderFunc functions are responsible for sending a
// response related to a charm bundle.
type bundleContentSenderFunc func(w http.ResponseWriter, r *http.Request, bundle *charm.CharmArchive) error

func (h *charmsHandler) ServeUnsupported(w http.ResponseWriter, r *http.Request) error {
	return errors.Trace(emitUnsupportedMethodErr(r.Method))
}

func (h *charmsHandler) ServePost(w http.ResponseWriter, r *http.Request) error {
	if h.logger.IsLevelEnabled(corelogger.TRACE) {
		h.logger.Tracef("ServePost(%s)", r.URL)
	}

	if r.Method != "POST" {
		return errors.Trace(emitUnsupportedMethodErr(r.Method))
	}

	// Make sure the content type is zip.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/zip" {
		return errors.BadRequestf("expected Content-Type: application/zip, got: %v", contentType)
	}

	st, err := h.stateAuthFunc(r)
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Release()

	// Add a charm to the store provider.
	charmURL, err := h.processPost(r, st.State)
	if err != nil {
		return errors.NewBadRequest(err, "")
	}
	return errors.Trace(sendStatusAndHeadersAndJSON(w, http.StatusOK, map[string]string{"Juju-Curl": charmURL}, &params.CharmsResponse{CharmURL: charmURL}))
}

func (h *charmsHandler) ServeGet(w http.ResponseWriter, r *http.Request) error {
	if h.logger.IsLevelEnabled(corelogger.TRACE) {
		h.logger.Tracef("ServeGet(%s)", r.URL)
	}

	if r.Method != "GET" {
		return errors.Trace(emitUnsupportedMethodErr(r.Method))
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
	charmArchivePath, fileArg, serveIcon, err := h.processGet(r, st.State)
	if err != nil {
		// An error occurred retrieving the charm bundle.
		if errors.Is(err, errors.NotFound) || errors.Is(err, errors.NotYetAvailable) {
			return errors.Trace(err)
		}

		return errors.NewBadRequest(err, "")
	}
	defer os.Remove(charmArchivePath)

	var sender bundleContentSenderFunc
	switch fileArg {
	case "":
		// The client requested the list of charm files.
		sender = h.manifestSender
	case "*":
		// The client requested the archive.
		sender = h.archiveSender
	default:
		// The client requested a specific file.
		sender = h.archiveEntrySender(fileArg, serveIcon)
	}

	return errors.Trace(sendBundleContent(w, r, charmArchivePath, sender))
}

// manifestSender sends a JSON-encoded response to the client including the
// list of files contained in the charm bundle.
func (h *charmsHandler) manifestSender(w http.ResponseWriter, r *http.Request, bundle *charm.CharmArchive) error {
	manifest, err := bundle.ArchiveMembers()
	if err != nil {
		return errors.Annotatef(err, "unable to read manifest in %q", bundle.Path)
	}
	return errors.Trace(sendStatusAndJSON(w, http.StatusOK, &params.CharmsResponse{
		Files: manifest.SortedValues(),
	}))
}

// archiveEntrySender returns a bundleContentSenderFunc which is responsible
// for sending the contents of filePath included in the given charm bundle. If
// filePath does not identify a file or a symlink, a 403 forbidden error is
// returned. If serveIcon is true, then the charm icon.svg file is sent, or a
// default icon if that file is not included in the charm.
func (h *charmsHandler) archiveEntrySender(filePath string, serveIcon bool) bundleContentSenderFunc {
	return func(w http.ResponseWriter, r *http.Request, bundle *charm.CharmArchive) error {
		contents, err := common.CharmArchiveEntry(bundle.Path, filePath, serveIcon)
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

// archiveSender is a bundleContentSenderFunc which is responsible for sending
// the contents of the given charm bundle.
func (h *charmsHandler) archiveSender(w http.ResponseWriter, r *http.Request, bundle *charm.CharmArchive) error {
	// Note that http.ServeFile's error responses are not our standard JSON
	// responses (they are the usual textual error messages as produced
	// by http.Error), but there's not a great deal we can do about that,
	// except accept non-JSON error responses in the client, because
	// http.ServeFile does not provide a way of customizing its
	// error responses.
	http.ServeFile(w, r, bundle.Path)
	return nil
}

// processPost handles a charm upload POST request after authentication.
func (h *charmsHandler) processPost(r *http.Request, st *state.State) (string, error) {
	query := r.URL.Query()
	schema := query.Get("schema")
	if schema == "" {
		schema = "local"
	}
	if schema != "local" {
		// charmhub charms may only be uploaded into models
		// which are being imported during model migrations.
		// There's currently no other time where it makes sense
		// to accept repository charms through this endpoint.
		if isImporting, err := modelIsImporting(st); err != nil {
			return "", errors.Trace(err)
		} else if !isImporting {
			return "", errors.New("charms may only be uploaded during model migration import")
		}
	}

	// Attempt to get the object store early, so we're not unnecessarily
	// creating a parsing/reading if we can't get the object store.
	objectStore, err := h.objectStoreGetter.GetObjectStore(r.Context(), st.ModelUUID())
	if err != nil {
		return "", errors.Trace(err)
	}

	charmFileName, err := writeCharmToTempFile(r.Body)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer os.Remove(charmFileName)

	err = h.processUploadedArchive(charmFileName)
	if err != nil {
		return "", err
	}
	archive, err := charm.ReadCharmArchive(charmFileName)
	if err != nil {
		return "", errors.BadRequestf("invalid charm archive: %v", err)
	}

	// Use the name from the query string. If we're dealing with an older client
	// then this won't be sent, instead fallback to the archive metadata name.
	name := query.Get("name")
	if name == "" {
		name = archive.Meta().Name
	}
	if err := charm.ValidateName(name); err != nil {
		return "", errors.NewBadRequest(err, "")
	}

	var revision int
	if revisionStr := query.Get("revision"); revisionStr != "" {
		revision, err = strconv.Atoi(revisionStr)
		if err != nil {
			return "", errors.NewBadRequest(errors.NewNotValid(err, "revision"), "")
		}
	} else {
		revision = archive.Revision()
	}

	// We got it, now let's reserve a charm URL for it in state.
	curlStr := curlString(schema, query.Get("arch"), name, query.Get("series"), revision)

	switch charm.Schema(schema) {
	case charm.Local:
		curl, err := st.PrepareLocalCharmUpload(curlStr)
		if err != nil {
			return "", errors.Trace(err)
		}
		curlStr = curl.String()

	case charm.CharmHub:
		if _, err := st.PrepareCharmUpload(curlStr); err != nil {
			return "", errors.Trace(err)
		}

	default:
		return "", errors.Errorf("unsupported schema %q", schema)
	}

	err = RepackageAndUploadCharm(r.Context(), objectStore, storageStateShim{State: st}, archive, curlStr, revision)
	if err != nil {
		return "", errors.Trace(err)
	}
	return curlStr, nil
}

// curlString takes the constituent parts of a charm url and renders the url as a string.
// This is required since, to support migrations from legacy controllers, we need to support
// charm urls with series since controllers do not allow migrations to mutate charm urls during
// migration.
//
// This is the only place in Juju 4 where series in a charm url needs to be processed. As such,
// instead of dragging support for series with us into 4.0, in this one place we string-hack the
// url
func curlString(schema, arch, name, series string, revision int) string {
	if series == "" {
		curl := &charm.URL{
			Schema:       schema,
			Architecture: arch,
			Name:         name,
			Revision:     revision,
		}
		return curl.String()
	}
	var curl string
	if arch == "" {
		curl = fmt.Sprintf("%s:%s/%s", schema, series, name)
	} else {
		curl = fmt.Sprintf("%s:%s/%s/%s", schema, arch, series, name)
	}
	if revision != -1 {
		curl = fmt.Sprintf("%s-%d", curl, revision)
	}
	return curl
}

// processUploadedArchive opens the given charm archive from path,
// inspects it to see if it has all files at the root of the archive
// or it has subdirs. It repackages the archive so it has all the
// files at the root dir, if necessary, replacing the original archive
// at path.
func (h *charmsHandler) processUploadedArchive(path string) error {
	// Open the archive as a zip.
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	zipr, err := zip.NewReader(f, fi.Size())
	if err != nil {
		return errors.Annotate(err, "cannot open charm archive")
	}

	// Find out the root dir prefix from the archive.
	rootDir, err := h.findArchiveRootDir(zipr)
	if err != nil {
		return errors.Annotate(err, "cannot read charm archive")
	}
	if rootDir == "." {
		// Normal charm, just use charm.ReadCharmArchive).
		return nil
	}

	// There is one or more subdirs, so we need extract it to a temp
	// dir and then read it as a charm dir.
	tempDir, err := os.MkdirTemp("", "charm-extract")
	if err != nil {
		return errors.Annotate(err, "cannot create temp directory")
	}
	defer os.RemoveAll(tempDir)
	if err := ziputil.Extract(zipr, tempDir, rootDir); err != nil {
		return errors.Annotate(err, "cannot extract charm archive")
	}
	dir, err := charm.ReadCharmDir(tempDir)
	if err != nil {
		return errors.Annotate(err, "cannot read extracted archive")
	}

	// Now repackage the dir as a bundle at the original path.
	if err := f.Truncate(0); err != nil {
		return err
	}
	if err := dir.ArchiveTo(f); err != nil {
		return err
	}
	return nil
}

// findArchiveRootDir scans a zip archive and returns the rootDir of
// the archive, the one containing metadata.yaml, config.yaml and
// revision files, or an error if the archive appears invalid.
func (h *charmsHandler) findArchiveRootDir(zipr *zip.Reader) (string, error) {
	paths, err := ziputil.Find(zipr, "metadata.yaml")
	if err != nil {
		return "", err
	}
	switch len(paths) {
	case 0:
		return "", errors.Errorf("invalid charm archive: missing metadata.yaml")
	case 1:
	default:
		sort.Sort(byDepth(paths))
		if depth(paths[0]) == depth(paths[1]) {
			return "", errors.Errorf("invalid charm archive: ambiguous root directory")
		}
	}
	return filepath.Dir(paths[0]), nil
}

func depth(path string) int {
	return strings.Count(path, "/")
}

type byDepth []string

func (d byDepth) Len() int           { return len(d) }
func (d byDepth) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d byDepth) Less(i, j int) bool { return depth(d[i]) < depth(d[j]) }

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
func RepackageAndUploadCharm(ctx context.Context, objectStore services.Storage, uploader CharmUploader, archive *charm.CharmArchive, curl string, charmRevision int) error {
	// Create a temp dir to contain the extracted charm dir.
	tempDir, err := os.MkdirTemp("", "charm-download")
	if err != nil {
		return errors.Annotate(err, "cannot create temp directory")
	}
	defer os.RemoveAll(tempDir)
	extractPath := filepath.Join(tempDir, "extracted")

	// Expand and repack it with the specified revision
	archive.SetRevision(charmRevision)
	if err := archive.ExpandTo(extractPath); err != nil {
		return errors.Annotate(err, "cannot extract uploaded charm")
	}

	charmDir, err := charm.ReadCharmDir(extractPath)
	if err != nil {
		return errors.Annotate(err, "cannot read extracted charm")
	}

	// Try to get the version details here.
	// read just the first line of the file.
	var version string
	versionPath := filepath.Join(extractPath, "version")
	if file, err := os.Open(versionPath); err == nil {
		version, err = charm.ReadVersion(file)
		_ = file.Close()
		if err != nil {
			return errors.Trace(err)
		}
	} else if !os.IsNotExist(err) {
		return errors.Annotate(err, "cannot open version file")
	}

	// Bundle the charm and calculate its sha256 hash at the same time.
	var repackagedArchive bytes.Buffer
	hash := sha256.New()
	err = charmDir.ArchiveTo(io.MultiWriter(hash, &repackagedArchive))
	if err != nil {
		return errors.Annotate(err, "cannot repackage uploaded charm")
	}
	bundleSHA256 := hex.EncodeToString(hash.Sum(nil))

	// Now we need to repackage it with the reserved URL, upload it to
	// provider storage and update the state.
	charmStorage := services.NewCharmStorage(services.CharmStorageConfig{
		Logger:       logger,
		StateBackend: uploader,
		ObjectStore:  objectStore,
	})

	return charmStorage.Store(ctx, curl, downloader.DownloadedCharm{
		Charm:        archive,
		CharmData:    &repackagedArchive,
		CharmVersion: version,
		Size:         int64(repackagedArchive.Len()),
		SHA256:       bundleSHA256,
		LXDProfile:   charmDir.LXDProfile(),
	})
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
// It returns the bundle path, the requested file path (if any), whether the
// default charm icon has been requested and an error.
func (h *charmsHandler) processGet(r *http.Request, st *state.State) (
	archivePath string,
	fileArg string,
	serveIcon bool,
	err error,
) {
	errRet := func(err error) (string, string, bool, error) {
		return "", "", false, err
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
	} else if query.Get("icon") == "1" {
		serveIcon = true
		fileArg = "icon.svg"
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
	return archivePath, fileArg, serveIcon, nil
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
		logger.Debugf("returning error from %s %s: %s", req.Method, req.URL, errors.Details(err))
	} else {
		logger.Errorf("returning error from %s %s: %s", req.Method, req.URL, errors.Details(err))
	}

	perr, status := apiservererrors.ServerErrorAndStatus(err)
	return errors.Trace(sendStatusAndJSON(w, status, &params.CharmsResponse{
		Error:     perr.Message,
		ErrorCode: perr.Code,
		ErrorInfo: perr.Info,
	}))
}

// sendBundleContent uses the given bundleContentSenderFunc to send a
// response related to the charm archive located in the given
// archivePath.
func sendBundleContent(
	w http.ResponseWriter,
	r *http.Request,
	archivePath string,
	sender bundleContentSenderFunc,
) error {
	logger.Child("charmhttp").Tracef("sendBundleContent %q", archivePath)
	bundle, err := charm.ReadCharmArchive(archivePath)
	if err != nil {
		return errors.Annotatef(err, "unable to read archive in %q", archivePath)
	}
	// The bundleContentSenderFunc will set up and send an appropriate response.
	if err := sender(w, r, bundle); err != nil {
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
