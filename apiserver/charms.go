// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/errors"
	ziputil "github.com/juju/utils/zip"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/application"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
)

// charmsHandler handles charm upload through HTTPS in the API server.
type charmsHandler struct {
	ctxt    httpContext
	dataDir string
}

// bundleContentSenderFunc functions are responsible for sending a
// response related to a charm bundle.
type bundleContentSenderFunc func(w http.ResponseWriter, r *http.Request, bundle *charm.CharmArchive) error

func (h *charmsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "POST":
		err = h.servePost(w, r)
	case "GET":
		err = h.serveGet(w, r)
	default:
		err = errors.MethodNotAllowedf("unsupported method: %q", r.Method)
	}
	if err != nil {
		h.sendError(w, r, err)
	}
}

func (h *charmsHandler) servePost(w http.ResponseWriter, r *http.Request) error {
	st, _, err := h.ctxt.stateForRequestAuthenticatedUser(r)
	if err != nil {
		return errors.Trace(err)
	}
	// Add a charm to the store provider.
	charmURL, err := h.processPost(r, st)
	if err != nil {
		return errors.NewBadRequest(err, "")
	}
	sendStatusAndJSON(w, http.StatusOK, &params.CharmsResponse{CharmURL: charmURL.String()})
	return nil
}

func (h *charmsHandler) serveGet(w http.ResponseWriter, r *http.Request) error {
	st, _, err := h.ctxt.stateForRequestAuthenticated(r)
	if err != nil {
		return errors.Trace(err)
	}
	// Retrieve or list charm files.
	// Requires "url" (charm URL) and an optional "file" (the path to the
	// charm file) to be included in the query.
	charmArchivePath, filePath, err := h.processGet(r, st)
	if err != nil {
		// An error occurred retrieving the charm bundle.
		if errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		return errors.NewBadRequest(err, "")
	}
	var sender bundleContentSenderFunc
	switch filePath {
	case "":
		// The client requested the list of charm files.
		sender = h.manifestSender
	case "*":
		// The client requested the archive.
		sender = h.archiveSender
	default:
		// The client requested a specific file.
		sender = h.archiveEntrySender(filePath)
	}
	if err := h.sendBundleContent(w, r, charmArchivePath, sender); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// sendError sends a JSON-encoded error response.
// Note the difference from the error response sent by
// the sendError function - the error is encoded in the
// Error field as a string, not an Error object.
func (h *charmsHandler) sendError(w http.ResponseWriter, req *http.Request, err error) {
	logger.Errorf("returning error from %s %s: %s", req.Method, req.URL, errors.Details(err))
	perr, status := common.ServerErrorAndStatus(err)
	sendStatusAndJSON(w, status, &params.CharmsResponse{
		Error:     perr.Message,
		ErrorCode: perr.Code,
		ErrorInfo: perr.Info,
	})
}

// sendBundleContent uses the given bundleContentSenderFunc to send a response
// related to the charm archive located in the given archivePath.
func (h *charmsHandler) sendBundleContent(w http.ResponseWriter, r *http.Request, archivePath string, sender bundleContentSenderFunc) error {
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

// manifestSender sends a JSON-encoded response to the client including the
// list of files contained in the charm bundle.
func (h *charmsHandler) manifestSender(w http.ResponseWriter, r *http.Request, bundle *charm.CharmArchive) error {
	manifest, err := bundle.Manifest()
	if err != nil {
		return errors.Annotatef(err, "unable to read manifest in %q", bundle.Path)
	}
	sendStatusAndJSON(w, http.StatusOK, &params.CharmsResponse{
		Files: manifest.SortedValues(),
	})
	return nil
}

// archiveEntrySender returns a bundleContentSenderFunc which is responsible for
// sending the contents of filePath included in the given charm bundle. If filePath
// does not identify a file or a symlink, a 403 forbidden error is returned.
func (h *charmsHandler) archiveEntrySender(filePath string) bundleContentSenderFunc {
	return func(w http.ResponseWriter, r *http.Request, bundle *charm.CharmArchive) error {
		// TODO(fwereade) 2014-01-27 bug #1285685
		// This doesn't handle symlinks helpfully, and should be talking in
		// terms of bundles rather than zip readers; but this demands thought
		// and design and is not amenable to a quick fix.
		zipReader, err := zip.OpenReader(bundle.Path)
		if err != nil {
			return errors.Annotatef(err, "unable to read charm")
		}
		defer zipReader.Close()
		for _, file := range zipReader.File {
			if path.Clean(file.Name) != filePath {
				continue
			}
			fileInfo := file.FileInfo()
			if fileInfo.IsDir() {
				return &params.Error{
					Message: "directory listing not allowed",
					Code:    params.CodeForbidden,
				}
			}
			contents, err := file.Open()
			if err != nil {
				return errors.Annotatef(err, "unable to read file %q", filePath)
			}
			defer contents.Close()
			ctype := mime.TypeByExtension(filepath.Ext(filePath))
			if ctype != "" {
				// Older mime.types may map .js to x-javascript.
				// Map it to javascript for consistency.
				if ctype == params.ContentTypeXJS {
					ctype = params.ContentTypeJS
				}
				w.Header().Set("Content-Type", ctype)
			}
			w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
			w.WriteHeader(http.StatusOK)
			io.Copy(w, contents)
			return nil
		}
		return errors.NotFoundf("charm")
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
	series := query.Get("series")

	// Make sure the content type is zip.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/zip" {
		return nil, fmt.Errorf("expected Content-Type: application/zip, got: %v", contentType)
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
		return nil, fmt.Errorf("invalid charm archive: %v", err)
	}

	// We got it, now let's reserve a charm URL for it in state.
	curl := &charm.URL{
		Schema:   schema,
		Name:     archive.Meta().Name,
		Revision: archive.Revision(),
		Series:   series,
	}
	if schema == "local" {
		curl, err = st.PrepareLocalCharmUpload(curl)
		if err != nil {
			return nil, err
		}
	} else {
		// "cs:" charms may only be uploaded into models which are
		// being imported during model migrations. There's currently
		// no other time where it makes sense to accept charm store
		// charms through this endpoint.
		if isImporting, err := modelIsImporting(st); err != nil {
			return nil, errors.Trace(err)
		} else if !isImporting {
			return nil, errors.New("cs charms may only be uploaded during model migration import")
		}

		// If a revision argument is provided, it takes precedence
		// over the revision in the charm archive. This is required to
		// handle the revision differences between unpublished and
		// published charms in the charm store.
		revisionStr := query.Get("revision")
		if revisionStr != "" {
			curl.Revision, err = strconv.Atoi(revisionStr)
			if err != nil {
				return nil, errors.NotValidf("revision")
			}
		}
		if _, err := st.PrepareStoreCharmUpload(curl); err != nil {
			return nil, errors.Trace(err)
		}
	}

	// Now we need to repackage it with the reserved URL, upload it to
	// provider storage and update the state.
	err = h.repackageAndUploadCharm(st, archive, curl)
	if err != nil {
		return nil, err
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
	tempDir, err := ioutil.TempDir("", "charm-extract")
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
		return "", fmt.Errorf("invalid charm archive: missing metadata.yaml")
	case 1:
	default:
		sort.Sort(byDepth(paths))
		if depth(paths[0]) == depth(paths[1]) {
			return "", fmt.Errorf("invalid charm archive: ambiguous root directory")
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

// repackageAndUploadCharm expands the given charm archive to a
// temporary directoy, repackages it with the given curl's revision,
// then uploads it to storage, and finally updates the state.
func (h *charmsHandler) repackageAndUploadCharm(st *state.State, archive *charm.CharmArchive, curl *charm.URL) error {
	// Create a temp dir to contain the extracted charm dir.
	tempDir, err := ioutil.TempDir("", "charm-download")
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

	// Bundle the charm and calculate its sha256 hash at the same time.
	var repackagedArchive bytes.Buffer
	hash := sha256.New()
	err = charmDir.ArchiveTo(io.MultiWriter(hash, &repackagedArchive))
	if err != nil {
		return errors.Annotate(err, "cannot repackage uploaded charm")
	}
	bundleSHA256 := hex.EncodeToString(hash.Sum(nil))

	info := application.CharmArchive{
		ID:     curl,
		Charm:  archive,
		Data:   &repackagedArchive,
		Size:   int64(repackagedArchive.Len()),
		SHA256: bundleSHA256,
	}
	// Store the charm archive in environment storage.
	return application.StoreCharmArchive(st, info)
}

// processGet handles a charm file GET request after authentication.
// It returns the bundle path, the requested file path (if any) and an error.
func (h *charmsHandler) processGet(r *http.Request, st *state.State) (string, string, error) {
	query := r.URL.Query()

	// Retrieve and validate query parameters.
	curlString := query.Get("url")
	if curlString == "" {
		return "", "", fmt.Errorf("expected url=CharmURL query argument")
	}
	curl, err := charm.ParseURL(curlString)
	if err != nil {
		return "", "", errors.Annotate(err, "cannot parse charm URL")
	}

	var filePath string
	file := query.Get("file")
	if file == "" {
		filePath = ""
	} else {
		filePath = path.Clean(file)
	}

	// Prepare the bundle directories.
	name := charm.Quote(curlString)
	charmArchivePath := filepath.Join(
		h.dataDir,
		"charm-get-cache",
		st.ModelUUID(),
		name+".zip",
	)

	// Check if the charm archive is already in the cache.
	if _, err := os.Stat(charmArchivePath); os.IsNotExist(err) {
		// Download the charm archive and save it to the cache.
		if err = h.downloadCharm(st, curl, charmArchivePath); err != nil {
			return "", "", errors.Annotate(err, "unable to retrieve and save the charm")
		}
	} else if err != nil {
		return "", "", errors.Annotate(err, "cannot access the charms cache")
	}
	return charmArchivePath, filePath, nil
}

// downloadCharm downloads the given charm name from the provider storage and
// saves the corresponding zip archive to the given charmArchivePath.
func (h *charmsHandler) downloadCharm(st *state.State, curl *charm.URL, charmArchivePath string) error {
	storage := storage.NewStorage(st.ModelUUID(), st.MongoSession())
	ch, err := st.Charm(curl)
	if err != nil {
		return errors.Annotate(err, "cannot get charm from state")
	}

	// In order to avoid races, the archive is saved in a temporary file which
	// is then atomically renamed. The temporary file is created in the
	// charm cache directory so that we can safely assume the rename source and
	// target live in the same file system.
	cacheDir := filepath.Dir(charmArchivePath)
	if err = os.MkdirAll(cacheDir, 0755); err != nil {
		return errors.Annotate(err, "cannot create the charms cache")
	}
	tempCharmArchive, err := ioutil.TempFile(cacheDir, "charm")
	if err != nil {
		return errors.Annotate(err, "cannot create charm archive temp file")
	}
	defer cleanupFile(tempCharmArchive)

	// Use the storage to retrieve and save the charm archive.
	reader, _, err := storage.Get(ch.StoragePath())
	if err != nil {
		return errors.Annotate(err, "cannot get charm from model storage")
	}
	defer reader.Close()
	if _, err = io.Copy(tempCharmArchive, reader); err != nil {
		return errors.Annotate(err, "error processing charm archive download")
	}
	tempCharmArchive.Close()

	// Note that os.Rename won't fail if the target already exists;
	// there's no problem if there's concurrent get requests for the
	// same charm.
	if err = os.Rename(tempCharmArchive.Name(), charmArchivePath); err != nil {
		return errors.Annotate(err, "error renaming the charm archive")
	}
	return nil
}

// On windows we cannot remove a file until it has been closed
// If this poses an active problem somewhere else it will be refactored in
// utils and used everywhere.
func cleanupFile(file *os.File) {
	// Errors are ignored because it is ok for this to be called when
	// the file is already closed or has been moved.
	file.Close()
	os.Remove(file.Name())
}

func writeCharmToTempFile(r io.Reader) (string, error) {
	tempFile, err := ioutil.TempFile("", "charm")
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
