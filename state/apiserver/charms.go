// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/errgo/errgo"

	"launchpad.net/juju-core/charm"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

// charmsHandler handles charm upload through HTTPS in the API server.
type charmsHandler struct {
	state   *state.State
	dataDir string
}

type zipContentsSenderFunc func(w http.ResponseWriter, r *http.Request, reader *zip.ReadCloser)

func (h *charmsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.authenticate(r); err != nil {
		h.authError(w)
		return
	}

	switch r.Method {
	case "POST":
		// Add a local charm to the store provider.
		// Requires a "series" query specifying the series to use for the charm.
		charmURL, err := h.processPost(r)
		if err != nil {
			h.sendError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.sendJSON(w, http.StatusOK, &params.CharmsResponse{CharmURL: charmURL.String()})
	case "GET":
		// Retrieve or list charm files.
		// Requires "url" (charm URL) and an optional "file" (the path to the
		// charm file) to be included in the query.
		if charmArchivePath, filePath, err := h.processGet(r); err != nil {
			// An error occurred retrieving the charm bundle.
			h.sendError(w, http.StatusBadRequest, err.Error())
		} else if filePath == "" {
			// The client requested the list of charm files.
			sendZipContents(w, r, charmArchivePath, h.fileListSender)
		} else {
			// The client requested a specific file.
			sendZipContents(w, r, charmArchivePath, h.fileSender(filePath))
		}
	default:
		h.sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", r.Method))
	}
}

// sendJSON sends a JSON-encoded response to the client.
func (h *charmsHandler) sendJSON(w http.ResponseWriter, statusCode int, response *params.CharmsResponse) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	w.Write(body)
	return nil
}

func sendZipContents(w http.ResponseWriter, r *http.Request, archivePath string, zipContentsSender zipContentsSenderFunc) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		http.Error(
			w, fmt.Sprintf("unable to read archive in %q: %v", archivePath, err),
			http.StatusInternalServerError)
		return
	}
	defer reader.Close()
	zipContentsSender(w, r, reader)
}

func (h *charmsHandler) fileListSender(w http.ResponseWriter, r *http.Request, reader *zip.ReadCloser) {
	var files []string
	for _, file := range reader.File {
		fileInfo := file.FileInfo()
		if !fileInfo.IsDir() {
			files = append(files, file.Name)
		}
	}
	h.sendJSON(w, http.StatusOK, &params.CharmsResponse{Files: files})
}

func (h *charmsHandler) fileSender(filePath string) zipContentsSenderFunc {
	return func(w http.ResponseWriter, r *http.Request, reader *zip.ReadCloser) {
		for _, file := range reader.File {
			if h.fixPath(file.Name) != filePath {
				continue
			}
			fileInfo := file.FileInfo()
			if fileInfo.IsDir() {
				http.Error(w, "directory listing not allowed", http.StatusForbidden)
				return
			}
			if contents, err := file.Open(); err != nil {
				http.Error(
					w, fmt.Sprintf("unable to read file %q: %v", filePath, err),
					http.StatusInternalServerError)
			} else {
				defer contents.Close()
				w.WriteHeader(http.StatusOK)
				io.Copy(w, contents)
			}
			return
		}
		http.NotFound(w, r)
		return
	}
}

// sendZipFilesList sends a JSON-encoded response to the client including the
// list of files contained in the zip archive present in the given archivePath.
func (h *charmsHandler) sendZipFilesList(w http.ResponseWriter, archivePath string) {
	var files []string
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		http.Error(
			w, fmt.Sprintf("unable to read archive in %q: %v", archivePath, err),
			http.StatusInternalServerError)
		return
	}
	defer reader.Close()
	for _, file := range reader.File {
		fileInfo := file.FileInfo()
		if !fileInfo.IsDir() {
			files = append(files, file.Name)
		}
	}
	h.sendJSON(w, http.StatusOK, &params.CharmsResponse{Files: files})
}

// sendZipFile sends the file contents of a file included in the given zip.
// A 404 page not found is returned if path does not exist.
// A 403 forbidden error is returned if path points to a directory.
func (h *charmsHandler) sendZipFile(w http.ResponseWriter, r *http.Request, archivePath, filePath string) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		http.Error(
			w, fmt.Sprintf("unable to read archive in %q: %v", archivePath, err),
			http.StatusInternalServerError)
		return
	}
	defer reader.Close()
	for _, file := range reader.File {
		if h.fixPath(file.Name) != filePath {
			continue
		}
		fileInfo := file.FileInfo()
		if fileInfo.IsDir() {
			http.Error(w, "directory listing not allowed", http.StatusForbidden)
			return
		}
		if contents, err := file.Open(); err != nil {
			http.Error(
				w, fmt.Sprintf("unable to read file %q: %v", filePath, err),
				http.StatusInternalServerError)
		} else {
			defer contents.Close()
			w.WriteHeader(http.StatusOK)
			io.Copy(w, contents)
		}
		return
	}
	http.NotFound(w, r)
	return
}

// sendError sends a JSON-encoded error response.
func (h *charmsHandler) sendError(w http.ResponseWriter, statusCode int, message string) error {
	return h.sendJSON(w, statusCode, &params.CharmsResponse{Error: message})
}

// authenticate parses HTTP basic authentication and authorizes the
// request by looking up the provided tag and password against state.
func (h *charmsHandler) authenticate(r *http.Request) error {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || parts[0] != "Basic" {
		// Invalid header format or no header provided.
		return fmt.Errorf("invalid request format")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	// See RFC 2617, Section 2.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid request format")
	}
	tagPass := strings.SplitN(string(challenge), ":", 2)
	if len(tagPass) != 2 {
		return fmt.Errorf("invalid request format")
	}
	entity, err := checkCreds(h.state, params.Creds{
		AuthTag:  tagPass[0],
		Password: tagPass[1],
	})
	if err != nil {
		return err
	}
	// Only allow users, not agents.
	_, _, err = names.ParseTag(entity.Tag(), names.UserTagKind)
	if err != nil {
		return common.ErrBadCreds
	}
	return err
}

// authError sends an unauthorized error.
func (h *charmsHandler) authError(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	h.sendError(w, http.StatusUnauthorized, "unauthorized")
}

// processPost handles a charm upload POST request after authentication.
func (h *charmsHandler) processPost(r *http.Request) (*charm.URL, error) {
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
	archive, err := charm.ReadBundle(tempFile.Name())
	if err != nil {
		return nil, fmt.Errorf("invalid charm archive: %v", err)
	}
	// We got it, now let's reserve a charm URL for it in state.
	archiveURL := &charm.URL{
		Schema:   "local",
		Series:   series,
		Name:     archive.Meta().Name,
		Revision: archive.Revision(),
	}
	preparedURL, err := h.state.PrepareLocalCharmUpload(archiveURL)
	if err != nil {
		return nil, err
	}
	// Now we need to repackage it with the reserved URL, upload it to
	// provider storage and update the state.
	err = h.repackageAndUploadCharm(archive, preparedURL)
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
		return errgo.Annotate(err, "cannot open charm archive")
	}

	// Find out the root dir prefix from the archive.
	rootDir, err := h.findArchiveRootDir(zipr)
	if err != nil {
		return errgo.Annotate(err, "cannot read charm archive")
	}
	if rootDir == "" {
		// Normal charm, just use charm.ReadBundle().
		return nil
	}
	// There is one or more subdirs, so we need extract it to a temp
	// dir and then read is as a charm dir.
	tempDir, err := ioutil.TempDir("", "charm-extract")
	if err != nil {
		return errgo.Annotate(err, "cannot create temp directory")
	}
	defer os.RemoveAll(tempDir)
	err = h.extractArchiveTo(zipr, rootDir, tempDir)
	if err != nil {
		return errgo.Annotate(err, "cannot extract charm archive")
	}
	dir, err := charm.ReadDir(tempDir)
	if err != nil {
		return errgo.Annotate(err, "cannot read extracted archive")
	}
	// Now repackage the dir as a bundle at the original path.
	if err := f.Truncate(0); err != nil {
		return err
	}
	if err := dir.BundleTo(f); err != nil {
		return err
	}
	return nil
}

// fixPath converts all forward and backslashes in path to the OS path
// separator and calls filepath.Clean before returning it.
func (h *charmsHandler) fixPath(path string) string {
	sep := string(filepath.Separator)
	p := strings.Replace(path, "\\", sep, -1)
	return filepath.Clean(strings.Replace(p, "/", sep, -1))
}

// findArchiveRootDir scans a zip archive and returns the rootDir of
// the archive, the one containing metadata.yaml, config.yaml and
// revision files, or an error if the archive appears invalid.
func (h *charmsHandler) findArchiveRootDir(zipr *zip.Reader) (string, error) {
	numFound := 0
	metadataFound := false // metadata.yaml is the only required file.
	rootPath := ""
	lookFor := []string{"metadata.yaml", "config.yaml", "revision"}
	for _, fh := range zipr.File {
		for _, fname := range lookFor {
			dir, file := filepath.Split(h.fixPath(fh.Name))
			if file == fname {
				if file == "metadata.yaml" {
					metadataFound = true
				}
				numFound++
				if rootPath == "" {
					rootPath = dir
				} else if rootPath != dir {
					return "", fmt.Errorf("invalid charm archive: expected all %v files in the same directory", lookFor)
				}
				if numFound == len(lookFor) {
					return rootPath, nil
				}
			}
		}
	}
	if !metadataFound {
		return "", fmt.Errorf("invalid charm archive: missing metadata.yaml")
	}
	return rootPath, nil
}

// extractArchiveTo extracts an archive to the given destDir, removing
// the rootDir from each file, effectively reducing any nested subdirs
// to the root level.
func (h *charmsHandler) extractArchiveTo(zipr *zip.Reader, rootDir, destDir string) error {
	for _, fh := range zipr.File {
		err := h.extractSingleFile(fh, rootDir, destDir)
		if err != nil {
			return err
		}
	}
	return nil
}

// extractSingleFile extracts the given zip file header, removing
// rootDir from the filename, to the destDir.
func (h *charmsHandler) extractSingleFile(fh *zip.File, rootDir, destDir string) error {
	cleanName := h.fixPath(fh.Name)
	relName, err := filepath.Rel(rootDir, cleanName)
	if err != nil {
		// Skip paths not relative to roo
		return nil
	}
	if strings.Contains(relName, "..") || relName == "." {
		// Skip current dir and paths outside rootDir.
		return nil
	}
	dirName := filepath.Dir(relName)
	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()

	mode := fh.Mode()
	destPath := filepath.Join(destDir, relName)
	if dirName != "" && mode&os.ModeDir != 0 {
		err = os.MkdirAll(destPath, mode&0777)
		if err != nil {
			return err
		}
		return nil
	}

	if mode&os.ModeSymlink != 0 {
		data, err := ioutil.ReadAll(f)
		if err != nil {
			return err
		}
		target := string(data)
		if filepath.IsAbs(target) {
			return fmt.Errorf("symlink %q is absolute: %q", cleanName, target)
		}
		p := filepath.Join(dirName, target)
		if strings.Contains(p, "..") {
			return fmt.Errorf("symlink %q links out of charm: %s", cleanName, target)
		}
		err = os.Symlink(target, destPath)
		if err != nil {
			return err
		}
	}
	if dirName == "hooks" {
		if mode&os.ModeType == 0 {
			// Set all hooks executable (by owner)
			mode = mode | 0100
		}
	}

	// Check file type.
	e := "file has an unknown type: %q"
	switch mode & os.ModeType {
	case os.ModeDir, os.ModeSymlink, 0:
		// That's expected, it's ok.
		e = ""
	case os.ModeNamedPipe:
		e = "file is a named pipe: %q"
	case os.ModeSocket:
		e = "file is a socket: %q"
	case os.ModeDevice:
		e = "file is a device: %q"
	}
	if e != "" {
		return fmt.Errorf(e, destPath)
	}

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, mode&0777)
	if err != nil {
		return fmt.Errorf("creating %q failed: %v", destPath, err)
	}
	defer out.Close()
	_, err = io.Copy(out, f)
	return err
}

// repackageAndUploadCharm expands the given charm archive to a
// temporary directoy, repackages it with the given curl's revision,
// then uploads it to providr storage, and finally updates the state.
func (h *charmsHandler) repackageAndUploadCharm(archive *charm.Bundle, curl *charm.URL) error {
	// Create a temp dir to contain the extracted charm
	// dir and the repackaged archive.
	tempDir, err := ioutil.TempDir("", "charm-download")
	if err != nil {
		return errgo.Annotate(err, "cannot create temp directory")
	}
	defer os.RemoveAll(tempDir)
	extractPath := filepath.Join(tempDir, "extracted")
	repackagedPath := filepath.Join(tempDir, "repackaged.zip")
	repackagedArchive, err := os.Create(repackagedPath)
	if err != nil {
		return errgo.Annotate(err, "cannot repackage uploaded charm")
	}
	defer repackagedArchive.Close()

	// Expand and repack it with the revision specified by curl.
	archive.SetRevision(curl.Revision)
	if err := archive.ExpandTo(extractPath); err != nil {
		return errgo.Annotate(err, "cannot extract uploaded charm")
	}
	charmDir, err := charm.ReadDir(extractPath)
	if err != nil {
		return errgo.Annotate(err, "cannot read extracted charm")
	}
	// Bundle the charm and calculate its sha256 hash at the
	// same time.
	hash := sha256.New()
	err = charmDir.BundleTo(io.MultiWriter(hash, repackagedArchive))
	if err != nil {
		return errgo.Annotate(err, "cannot repackage uploaded charm")
	}
	bundleSHA256 := hex.EncodeToString(hash.Sum(nil))
	size, err := repackagedArchive.Seek(0, 2)
	if err != nil {
		return errgo.Annotate(err, "cannot get charm file size")
	}
	// Seek to the beginning so the subsequent Put will read
	// the whole file again.
	if _, err := repackagedArchive.Seek(0, 0); err != nil {
		return errgo.Annotate(err, "cannot rewind the charm file reader")
	}

	// Now upload to provider storage.
	storage, err := envtesting.GetEnvironStorage(h.state)
	if err != nil {
		return errgo.Annotate(err, "cannot access provider storage")
	}
	name := charm.Quote(curl.String())
	if err := storage.Put(name, repackagedArchive, size); err != nil {
		return errgo.Annotate(err, "cannot upload charm to provider storage")
	}
	storageURL, err := storage.URL(name)
	if err != nil {
		return errgo.Annotate(err, "cannot get storage URL for charm")
	}
	bundleURL, err := url.Parse(storageURL)
	if err != nil {
		return errgo.Annotate(err, "cannot parse storage URL")
	}

	// And finally, update state.
	_, err = h.state.UpdateUploadedCharm(archive, curl, bundleURL, bundleSHA256)
	if err != nil {
		return errgo.Annotate(err, "cannot update uploaded charm in state")
	}
	return nil
}

// processGet handles a charm file download GET request after authentication.
// It returns the bundle path, the requested file path (if any) and an error.
func (h *charmsHandler) processGet(r *http.Request) (string, string, error) {
	query := r.URL.Query()

	// Retrieve and validate query parameters.
	curl := query.Get("url")
	if curl == "" {
		return "", "", fmt.Errorf("expected url=CharmURL query argument")
	}
	var filePath string
	file := query.Get("file")
	if file == "" {
		filePath = ""
	} else {
		filePath = h.fixPath(file)
	}

	// Prepare the bundle directories.
	name := charm.Quote(curl)
	charmArchivePath := filepath.Join(h.dataDir, "charm-get-cache", name+".zip")

	// Check if the charm archive is already in the cache.
	if _, err := os.Stat(charmArchivePath); os.IsNotExist(err) {
		// Download the charm archive and save it to the cache.
		if err = h.downloadCharm(name, charmArchivePath); err != nil {
			return "", "", fmt.Errorf("unable to retrieve and save the charm: %v", err)
		}
	} else if err != nil {
		return "", "", fmt.Errorf("cannot access the charms cache: %v", err)
	}
	return charmArchivePath, filePath, nil
}

// getFilePath return the absolute path of a charm file, based on the given
// bundlePath. It also checks that the resulting path lives inside the bundle.
// func (h *charmsHandler) getFilePath(bundlePath, file string) (string, error) {
// 	if file == "" {
// 		return "", nil
// 	}
// 	filePath, err := filepath.Abs(filepath.Join(bundlePath, file))
// 	if err != nil {
// 		return "", errgo.Annotate(err, "cannot retrieve the requested path")
// 	}
// 	if !strings.HasPrefix(filePath, bundlePath+"/") {
// 		return "", fmt.Errorf("invalid file path: %q", file)
// 	}
// 	return filePath, nil
// }

// downloadCharm downloads the given charm name from the provider storage and
// save the corresponding zip archive to the given charmArchivePath.
func (h *charmsHandler) downloadCharm(name, charmArchivePath string) error {
	// Get the provider storage.
	storage, err := envtesting.GetEnvironStorage(h.state)
	if err != nil {
		return errgo.Annotate(err, "cannot access provider storage")
	}

	// Use the storage to retrieve and save the charm archive.
	reader, err := storage.Get(name)
	if err != nil {
		return errgo.Annotate(err, "charm not found in the provider storage")
	}
	defer reader.Close()
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return errgo.Annotate(err, "cannot read charm data")
	}
	// In order to avoid races, the archive is saved in a temporary file which
	// is then atomically renamed. The temporary file is created in the
	// charm cache directory so that we can safely assume the rename source and
	// target live in the same file system.
	cacheDir := filepath.Dir(charmArchivePath)
	if err = os.MkdirAll(cacheDir, 0755); err != nil {
		return errgo.Annotate(err, "cannot create the charms cache")
	}
	tempCharmArchive, err := ioutil.TempFile(cacheDir, "charm")
	if err != nil {
		return errgo.Annotate(err, "cannot create charm archive temp file")
	}
	defer tempCharmArchive.Close()
	if err = ioutil.WriteFile(tempCharmArchive.Name(), data, 0644); err != nil {
		return errgo.Annotate(err, "error processing charm archive download")
	}
	if err = os.Rename(tempCharmArchive.Name(), charmArchivePath); err != nil {
		return errgo.Annotate(err, "error renaming the charm archive")
	}
	return nil

	// // Read and expand the charm bundle.
	// bundle, err := charm.ReadBundle(tempCharm.Name())
	// if err != nil {
	// 	return errgo.Annotate(err, "cannot read the charm bundle")
	// }
	// // In order to avoid races, the bundle is expanded in a temporary dir which
	// // is then atomically renamed. The temporary directory is created in the
	// // charm cache so that we can safely assume the rename source and target
	// // live in the same file system.
	// cacheDir, _ := filepath.Split(bundlePath)
	// if err = os.MkdirAll(cacheDir, 0755); err != nil {
	// 	return errgo.Annotate(err, "cannot create the charms cache")
	// }
	// bundleTempPath, err := ioutil.TempDir(cacheDir, "bundle")
	// if err != nil {
	// 	return errgo.Annotate(err, "cannot create the temporary bundle directory")
	// }
	// if err = bundle.ExpandTo(bundleTempPath); err != nil {
	// 	defer os.RemoveAll(bundleTempPath)
	// 	return errgo.Annotate(err, "error expanding the bundle")
	// }
	// if err = os.Rename(bundleTempPath, bundlePath); err != nil {
	// 	return errgo.Annotate(err, "error renaming the bundle")
	// }
	// return nil
}
