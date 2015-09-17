// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	"gopkg.in/juju/charm.v5"

	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/service"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
)

// charmsHandler handles charm upload through HTTPS in the API server.
type charmsHandler struct {
	httpHandler
	dataDir string
}

// bundleContentSenderFunc functions are responsible for sending a
// response related to a charm bundle.
type bundleContentSenderFunc func(w http.ResponseWriter, r *http.Request, bundle *charm.CharmArchive)

func (h *charmsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	stateWrapper, err := h.validateEnvironUUID(r)
	if err != nil {
		h.sendError(w, http.StatusNotFound, err.Error())
		return
	}

	switch r.Method {
	case "POST":
		if err := stateWrapper.authenticateUser(r); err != nil {
			h.authError(w, h)
			return
		}
		// Add a local charm to the store provider.
		// Requires a "series" query specifying the series to use for the charm.
		charmURL, err := h.processPost(r, stateWrapper.state)
		if err != nil {
			h.sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.sendJSON(w, http.StatusOK, &params.CharmsResponse{CharmURL: charmURL.String()})
	case "GET":
		// Retrieve or list charm files.
		// Requires "url" (charm URL) and an optional "file" (the path to the
		// charm file) to be included in the query.
		if charmArchivePath, filePath, err := h.processGet(r, stateWrapper.state); err != nil {
			// An error occurred retrieving the charm bundle.
			if errors.IsNotFound(err) {
				h.sendError(w, http.StatusNotFound, err.Error())
			} else {
				h.sendError(w, http.StatusBadRequest, err.Error())
			}
		} else if filePath == "" {
			// The client requested the list of charm files.
			sendBundleContent(w, r, charmArchivePath, h.manifestSender)
		} else if filePath == "*" {
			// The client requested the archive.
			sendBundleContent(w, r, charmArchivePath, h.archiveSender)
		} else {
			// The client requested a specific file.
			sendBundleContent(w, r, charmArchivePath, h.archiveEntrySender(filePath))
		}
	default:
		h.sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", r.Method))
	}
}

// sendJSON sends a JSON-encoded response to the client.
func (h *charmsHandler) sendJSON(w http.ResponseWriter, statusCode int, response *params.CharmsResponse) error {
	w.Header().Set("Content-Type", apihttp.CTypeJSON)
	w.WriteHeader(statusCode)
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	w.Write(body)
	return nil
}

// sendBundleContent uses the given bundleContentSenderFunc to send a response
// related to the charm archive located in the given archivePath.
func sendBundleContent(w http.ResponseWriter, r *http.Request, archivePath string, sender bundleContentSenderFunc) {
	bundle, err := charm.ReadCharmArchive(archivePath)
	if err != nil {
		http.Error(
			w, fmt.Sprintf("unable to read archive in %q: %v", archivePath, err),
			http.StatusInternalServerError)
		return
	}
	// The bundleContentSenderFunc will set up and send an appropriate response.
	sender(w, r, bundle)
}

// manifestSender sends a JSON-encoded response to the client including the
// list of files contained in the charm bundle.
func (h *charmsHandler) manifestSender(w http.ResponseWriter, r *http.Request, bundle *charm.CharmArchive) {
	manifest, err := bundle.Manifest()
	if err != nil {
		http.Error(
			w, fmt.Sprintf("unable to read archive in %q: %v", bundle.Path, err),
			http.StatusInternalServerError)
		return
	}
	h.sendJSON(w, http.StatusOK, &params.CharmsResponse{Files: manifest.SortedValues()})
}

// archiveEntrySender returns a bundleContentSenderFunc which is responsible for
// sending the contents of filePath included in the given charm bundle. If filePath
// does not identify a file or a symlink, a 403 forbidden error is returned.
func (h *charmsHandler) archiveEntrySender(filePath string) bundleContentSenderFunc {
	return func(w http.ResponseWriter, r *http.Request, bundle *charm.CharmArchive) {
		// TODO(fwereade) 2014-01-27 bug #1285685
		// This doesn't handle symlinks helpfully, and should be talking in
		// terms of bundles rather than zip readers; but this demands thought
		// and design and is not amenable to a quick fix.
		zipReader, err := zip.OpenReader(bundle.Path)
		if err != nil {
			http.Error(
				w, fmt.Sprintf("unable to read charm: %v", err),
				http.StatusInternalServerError)
			return
		}
		defer zipReader.Close()
		for _, file := range zipReader.File {
			if path.Clean(file.Name) != filePath {
				continue
			}
			fileInfo := file.FileInfo()
			if fileInfo.IsDir() {
				http.Error(w, "directory listing not allowed", http.StatusForbidden)
				return
			}
			contents, err := file.Open()
			if err != nil {
				http.Error(
					w, fmt.Sprintf("unable to read file %q: %v", filePath, err),
					http.StatusInternalServerError)
				return
			}
			defer contents.Close()
			ctype := mime.TypeByExtension(filepath.Ext(filePath))
			if ctype != "" {
				w.Header().Set("Content-Type", ctype)
			}
			w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
			w.WriteHeader(http.StatusOK)
			io.Copy(w, contents)
			return
		}
		http.NotFound(w, r)
		return
	}
}

// archiveSender is a bundleContentSenderFunc which is responsible for sending
// the contents of the given charm bundle.
func (h *charmsHandler) archiveSender(w http.ResponseWriter, r *http.Request, bundle *charm.CharmArchive) {
	http.ServeFile(w, r, bundle.Path)
}

// sendError sends a JSON-encoded error response.
func (h *charmsHandler) sendError(w http.ResponseWriter, statusCode int, message string) {
	if err := h.sendJSON(w, statusCode, &params.CharmsResponse{Error: message}); err != nil {
		logger.Errorf("failed to send error: %v", err)
	}
}

// processPost handles a charm upload POST request after authentication.
func (h *charmsHandler) processPost(r *http.Request, st *state.State) (*charm.URL, error) {
	query := r.URL.Query()
	series := query.Get("series")
	if series == "" {
		return nil, fmt.Errorf("expected series=URL argument")
	}
	// Make sure the content type is zip.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/zip" {
		return nil, fmt.Errorf("expected Content-Type: application/zip, got: %v", contentType)
	}
	tempFile, err := ioutil.TempFile("", "charm")
	if err != nil {
		return nil, fmt.Errorf("cannot create temp file: %v", err)
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	if _, err := io.Copy(tempFile, r.Body); err != nil {
		return nil, fmt.Errorf("error processing file upload: %v", err)
	}
	err = h.processUploadedArchive(tempFile.Name())
	if err != nil {
		return nil, err
	}
	archive, err := charm.ReadCharmArchive(tempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("invalid charm archive: %v", err)
	}
	// We got it, now let's reserve a charm URL for it in state.
	archiveURL := &charm.URL{
		Schema:   "local",
		Name:     archive.Meta().Name,
		Revision: archive.Revision(),
		Series:   series,
	}
	preparedURL, err := st.PrepareLocalCharmUpload(archiveURL)
	if err != nil {
		return nil, err
	}
	// Now we need to repackage it with the reserved URL, upload it to
	// provider storage and update the state.
	err = h.repackageAndUploadCharm(st, archive, preparedURL)
	if err != nil {
		return nil, err
	}
	// All done.
	return preparedURL, nil
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

	// Store the charm archive in environment storage.
	return service.StoreCharmArchive(
		st,
		curl,
		archive,
		&repackagedArchive,
		int64(repackagedArchive.Len()),
		bundleSHA256,
	)
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
	charmArchivePath := filepath.Join(h.dataDir, "charm-get-cache", name+".zip")

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
	storage := storage.NewStorage(st.EnvironUUID(), st.MongoSession())
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
	defer tempCharmArchive.Close()

	// Use the storage to retrieve and save the charm archive.
	reader, _, err := storage.Get(ch.StoragePath())
	if err != nil {
		defer cleanupFile(tempCharmArchive)
		return errors.Annotate(err, "cannot get charm from environment storage")
	}
	defer reader.Close()

	if _, err = io.Copy(tempCharmArchive, reader); err != nil {
		defer cleanupFile(tempCharmArchive)
		return errors.Annotate(err, "error processing charm archive download")
	}
	tempCharmArchive.Close()
	if err = os.Rename(tempCharmArchive.Name(), charmArchivePath); err != nil {
		defer cleanupFile(tempCharmArchive)
		return errors.Annotate(err, "error renaming the charm archive")
	}
	return nil
}

// On windows we cannot remove a file until it has been closed
// If this poses an active problem somewhere else it will be refactored in
// utils and used everywhere.
func cleanupFile(file *os.File) {
	file.Close()
	os.Remove(file.Name())
}
