// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	ziputil "github.com/juju/utils/v3/zip"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/core/charm/downloader"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
)

type FailableHandlerFunc func(http.ResponseWriter, *http.Request) error

// CharmsHTTPHandler creates is a http.Handler which serves POST
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
type CharmsHTTPHandler struct {
	PostHandler FailableHandlerFunc
	GetHandler  FailableHandlerFunc
}

func (h *CharmsHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "POST":
		err = errors.Annotate(h.PostHandler(w, r), "cannot upload charm")
	case "GET":
		err = errors.Annotate(h.GetHandler(w, r), "cannot retrieve charm")
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
	ctxt          httpContext
	dataDir       string
	stateAuthFunc func(*http.Request) (*state.PooledState, error)
}

// bundleContentSenderFunc functions are responsible for sending a
// response related to a charm bundle.
type bundleContentSenderFunc func(w http.ResponseWriter, r *http.Request, bundle *charm.CharmArchive) error

func (h *charmsHandler) ServeUnsupported(w http.ResponseWriter, r *http.Request) error {
	return errors.Trace(emitUnsupportedMethodErr(r.Method))
}

func (h *charmsHandler) ServePost(w http.ResponseWriter, r *http.Request) error {
	logger.Child("charmsHandler").Tracef("ServePost(%s)", r.URL)
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
	return errors.Trace(sendStatusAndJSON(w, http.StatusOK, &params.CharmsResponse{CharmURL: charmURL.String()}))
}

func (h *charmsHandler) ServeGet(w http.ResponseWriter, r *http.Request) error {
	logger.Child("charmsHandler").Tracef("ServeGet(%s)", r.URL)
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
		if errors.IsNotFound(err) || errors.IsNotYetAvailable(err) {
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
func (h *charmsHandler) processPost(r *http.Request, st *state.State) (*charm.URL, error) {
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
			return nil, errors.Trace(err)
		} else if !isImporting {
			return nil, errors.New("charms may only be uploaded during model migration import")
		}
	}

	series := query.Get("series")
	if series != "" {
		if err := charm.ValidateSeries(series); err != nil {
			return nil, errors.NewBadRequest(err, "")
		}
	}

	charmFileName, err := writeCharmToTempFile(r.Body)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer os.Remove(charmFileName)

	err = h.processUploadedArchive(charmFileName)
	if err != nil {
		return nil, err
	}
	archive, err := charm.ReadCharmArchive(charmFileName)
	if err != nil {
		return nil, errors.BadRequestf("invalid charm archive: %v", err)
	}

	// Use the name from the query string. If we're dealing with an older client
	// then this won't be sent, instead fallback to the archive metadata name.
	name := query.Get("name")
	if name == "" {
		name = archive.Meta().Name
	}
	if err := charm.ValidateName(name); err != nil {
		return nil, errors.NewBadRequest(err, "")
	}

	// We got it, now let's reserve a charm URL for it in state.
	curl := &charm.URL{
		Schema:       schema,
		Architecture: query.Get("arch"),
		Name:         name,
		Revision:     archive.Revision(),
		Series:       series,
	}

	switch charm.Schema(schema) {
	case charm.Local:
		curl, err = st.PrepareLocalCharmUpload(curl.String())
		if err != nil {
			return nil, errors.Trace(err)
		}

	case charm.CharmHub:
		// If a revision argument is provided, it takes precedence
		// over the revision in the charm archive. This is required to
		// handle the revision differences between unpublished and
		// published charms in the charm store.
		revisionStr := query.Get("revision")
		if revisionStr != "" {
			curl.Revision, err = strconv.Atoi(revisionStr)
			if err != nil {
				return nil, errors.NewBadRequest(errors.NewNotValid(err, "revision"), "")
			}
		}
		if _, err := st.PrepareCharmUpload(curl.String()); err != nil {
			return nil, errors.Trace(err)
		}

	default:
		return nil, errors.Errorf("unsupported schema %q", schema)
	}

	err = RepackageAndUploadCharm(st, archive, curl)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return curl, nil
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

// RepackageAndUploadCharm expands the given charm archive to a
// temporary directory, repackages it with the given curl's revision,
// then uploads it to storage, and finally updates the state.
func RepackageAndUploadCharm(st *state.State, archive *charm.CharmArchive, curl *charm.URL) error {
	// Create a temp dir to contain the extracted charm dir.
	tempDir, err := os.MkdirTemp("", "charm-download")
	if err != nil {
		return errors.Annotate(err, "cannot create temp directory")
	}
	defer os.RemoveAll(tempDir)
	extractPath := filepath.Join(tempDir, "extracted")

	// Expand and repack it with the revision specified by curl.
	archive.SetRevision(curl.Revision)
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
		StateBackend: storageStateShim{st},
		StorageFactory: func(modelUUID string) services.Storage {
			return storage.NewStorage(modelUUID, st.MongoSession())
		},
	})

	return charmStorage.Store(curl.String(), downloader.DownloadedCharm{
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

	store := storage.NewStorage(st.ModelUUID(), st.MongoSession())
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

	archivePath, err = common.ReadCharmFromStorage(store, h.dataDir, ch.StoragePath())
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
	if errors.IsNotYetAvailable(err) {
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
