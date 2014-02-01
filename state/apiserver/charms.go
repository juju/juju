// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"launchpad.net/juju-core/charm"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
)

// charmsHandler handles charm upload through HTTPS in the API server.
type charmsHandler struct {
	commonHandler
}

// newCharmsHandler creates a new charms handler.
func newCharmsHandler(state *state.State) *charmsHandler {
	return &charmsHandler{commonHandler{state}}
}

func (h *charmsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.authenticate(r); err != nil {
		h.sendAuthError(h, w)
		return
	}

	switch r.Method {
	case "POST":
		charmURL, err := h.processPost(r)
		if err != nil {
			h.sendError(h, w, http.StatusBadRequest, err.Error())
			return
		}
		h.sendJSON(w, http.StatusOK, &params.CharmsResponse{CharmURL: charmURL.String()})
	// Possible future extensions, like GET.
	default:
		h.sendError(h, w, http.StatusMethodNotAllowed, "unsupported method: %q", r.Method)
	}
}

// processPost handles a charm upload POST request after authentication.
func (h *charmsHandler) processPost(r *http.Request) (*charm.URL, error) {
	query := r.URL.Query()
	series := query.Get("series")
	if series == "" {
		return nil, fmt.Errorf("expected series= URL argument")
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
		return fmt.Errorf("cannot open charm archive: %v", err)
	}

	// Find out the root dir prefix from the archive.
	rootDir, err := h.findArchiveRootDir(zipr)
	if err != nil {
		return fmt.Errorf("cannot read charm archive: %v", err)
	}
	if rootDir == "" {
		// Normal charm, just use charm.ReadBundle().
		return nil
	}
	// There is one or more subdirs, so we need extract it to a temp
	// dir and then read is as a charm dir.
	tempDir, err := ioutil.TempDir("", "charm-extract")
	if err != nil {
		return fmt.Errorf("cannot create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)
	err = h.extractArchiveTo(zipr, rootDir, tempDir)
	if err != nil {
		return fmt.Errorf("cannot extract charm archive: %v", err)
	}
	dir, err := charm.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("cannot read extracted archive: %v", err)
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
		return fmt.Errorf("cannot create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)
	extractPath := filepath.Join(tempDir, "extracted")
	repackagedPath := filepath.Join(tempDir, "repackaged.zip")
	repackagedArchive, err := os.Create(repackagedPath)
	if err != nil {
		return fmt.Errorf("cannot repackage uploaded charm: %v", err)
	}
	defer repackagedArchive.Close()

	// Expand and repack it with the revision specified by curl.
	archive.SetRevision(curl.Revision)
	if err := archive.ExpandTo(extractPath); err != nil {
		return fmt.Errorf("cannot extract uploaded charm: %v", err)
	}
	charmDir, err := charm.ReadDir(extractPath)
	if err != nil {
		return fmt.Errorf("cannot read extracted charm: %v", err)
	}
	// Bundle the charm and calculate its sha256 hash at the
	// same time.
	hash := sha256.New()
	err = charmDir.BundleTo(io.MultiWriter(hash, repackagedArchive))
	if err != nil {
		return fmt.Errorf("cannot repackage uploaded charm: %v", err)
	}
	bundleSHA256 := hex.EncodeToString(hash.Sum(nil))
	size, err := repackagedArchive.Seek(0, 2)
	if err != nil {
		return fmt.Errorf("cannot get charm file size: %v", err)
	}
	// Seek to the beginning so the subsequent Put will read
	// the whole file again.
	if _, err := repackagedArchive.Seek(0, 0); err != nil {
		return fmt.Errorf("cannot rewind the charm file reader: %v", err)
	}

	// Now upload to provider storage.
	storage, err := envtesting.GetEnvironStorage(h.state)
	if err != nil {
		return fmt.Errorf("cannot access provider storage: %v", err)
	}
	name := charm.Quote(curl.String())
	if err := storage.Put(name, repackagedArchive, size); err != nil {
		return fmt.Errorf("cannot upload charm to provider storage: %v", err)
	}
	storageURL, err := storage.URL(name)
	if err != nil {
		return fmt.Errorf("cannot get storage URL for charm: %v", err)
	}
	bundleURL, err := url.Parse(storageURL)
	if err != nil {
		return fmt.Errorf("cannot parse storage URL: %v", err)
	}

	// And finally, update state.
	_, err = h.state.UpdateUploadedCharm(archive, curl, bundleURL, bundleSHA256)
	if err != nil {
		return fmt.Errorf("cannot update uploaded charm in state: %v", err)
	}
	return nil
}

// errorResponse wraps the message for an error response.
func (h *charmsHandler) errorResponse(message string) interface{} {
	return &params.CharmsResponse{Error: message}
}
