// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
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
