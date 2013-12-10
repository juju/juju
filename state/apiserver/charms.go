// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

// charmsHandler handles charm upload through HTTPS in the API server.
type charmsHandler struct {
	state *state.State
}

// CharmsResponse is the server response to a charm upload request.
type CharmsResponse struct {
	Code     int    `json:"code,omitempty"`
	Error    string `json:"error,omitempty"`
	CharmURL string `json:"charmUrl,omitempty"`
}

// sendJSON sends a JSON-encoded response to the client.
func (h *charmsHandler) sendJSON(w http.ResponseWriter, response *CharmsResponse) error {
	if response == nil {
		return fmt.Errorf("response is nil")
	}
	w.WriteHeader(response.Code)
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	w.Write(body)
	return nil
}

// sendError sends a JSON-encoded error response.
func (h *charmsHandler) sendError(w http.ResponseWriter, code int, message string) error {
	if code == 0 {
		// Use code 400 by default.
		code = http.StatusBadRequest
	} else if code == http.StatusOK {
		// Dont' report 200 OK.
		code = 0
	}
	err := h.sendJSON(w, &CharmsResponse{Code: code, Error: message})
	if err != nil {
		return err
	}
	return nil
}

// authenticate parses HTTP basic authentication and authorizes the
// request by looking up the provided tag and password against state.
func (h *charmsHandler) authenticate(w http.ResponseWriter, r *http.Request) error {
	if r == nil {
		return fmt.Errorf("invalid request")
	}
	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || parts[0] != "Basic" {
		// Invalid header format or no header provided.
		return fmt.Errorf("invalid request format")
	}
	// Challenge is a base64-encoded "tag:pass" string.
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
func (h *charmsHandler) processPost(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	series := query.Get("series")
	if series == "" {
		h.sendError(w, 0, "expected series= URL argument")
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		h.sendError(w, 0, err.Error())
		return
	}
	// Get the first (and hopefully only) uploaded part to process.
	part, err := reader.NextPart()
	if err == io.EOF {
		h.sendError(w, 0, "expected a single uploaded file, got none")
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("cannot process uploaded file: %v", err), http.StatusBadRequest)
		return
	}
	// Make sure the content type is zip.
	contentType := part.Header.Get("Content-Type")
	if contentType != "application/zip" {
		h.sendError(w, 0, fmt.Sprintf("expected Content-Type: application/zip, got: %v", contentType))
		return
	}
	tempFile, err := ioutil.TempFile("", "charm")
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, fmt.Sprintf("cannot create temp file: %v", err))
		return
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	if _, err := io.Copy(tempFile, part); err != nil {
		h.sendError(w, http.StatusInternalServerError, fmt.Sprintf("error processing file upload: %v", err))
		return
	}
	if _, err := reader.NextPart(); err != io.EOF {
		h.sendError(w, 0, "expected a single uploaded file, got more")
		return
	}
	archive, err := charm.ReadBundle(tempFile.Name())
	if err != nil {
		h.sendError(w, 0, fmt.Sprintf("invalid charm archive: %v", err))
		return
	}
	// We got it, now let's reserve a charm URL for it in state.
	preparedUrl := fmt.Sprintf("local:%s/%s-%d", series, archive.Meta().Name, archive.Revision())
	archiveUrl := charm.MustParseURL(preparedUrl)
	preparedCharm, err := h.state.PrepareCharmUpload(archiveUrl)
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Now we need to repackage it with the reserved URL, upload it to
	// provider storage and update the state.
	err = h.repackageAndUploadCharm(archive, preparedCharm.URL())
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// All done.
	h.sendJSON(w, &CharmsResponse{
		Code:     http.StatusOK,
		CharmURL: preparedCharm.URL().String(),
	})
}

// repackageAndUploadCharm expands the given charm archive to a
// temporary directoy, repackages it with the given curl's revision,
// then uploads it to providr storage, and finally updates the state.
func (h *charmsHandler) repackageAndUploadCharm(archive *charm.Bundle, curl *charm.URL) error {
	// Create a temp dir and file to use below.
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d", archive.Meta().Name, rand.Int()))
	tempFile, err := ioutil.TempFile("", "charm")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %v", err)
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	// Expand and repack it with the revision specified by curl.
	archive.SetRevision(curl.Revision)
	if err := archive.ExpandTo(tempDir); err != nil {
		return fmt.Errorf("cannot extract uploaded charm: %v", err)
	}
	defer os.RemoveAll(tempDir)
	charmDir, err := charm.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("cannot read extracted charm: %v", err)
	}
	if err := charmDir.BundleTo(tempFile); err != nil {
		return fmt.Errorf("cannot repackage uploaded charm: %v", err)
	}

	// Calculate the SHA256 hash.
	bundleSha256, size, err := getSha256(tempFile)
	if err != nil {
		return fmt.Errorf("cannot calculate charm SHA-256: %v", err)
	}

	// Now upload to provider storage.
	storage, err := getEnvironStorage(h.state)
	if err != nil {
		return fmt.Errorf("cannot access provider storage: %v", err)
	}
	name := charm.Quote(curl.String())
	if err := storage.Put(name, tempFile, size); err != nil {
		return fmt.Errorf("cannot upload charm: %v", err)
	}
	storageUrl, err := storage.URL(name)
	if err != nil {
		return fmt.Errorf("cannot get storage URL for charm: %v", err)
	}
	bundleURL, err := url.Parse(storageUrl)
	if err != nil {
		return fmt.Errorf("cannot parse storage URL: %v", err)
	}

	// And finally, update state.
	_, err = h.state.UpdateUploadedCharm(archive, curl, bundleURL, bundleSha256)
	if err != nil {
		return fmt.Errorf("cannot upload charm to storage: %v", err)
	}
	return nil
}

// getSha256 calculates the SHA-256 hash of the contents of source and
// returns it as a hex-encoded string, along with the source size in
// bytes.
func getSha256(source io.ReadSeeker) (string, int64, error) {
	hash := sha256.New()
	size, err := io.Copy(hash, source)
	if err != nil {
		return "", 0, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if _, err := source.Seek(0, 0); err != nil {
		return "", 0, err
	}
	return digest, size, nil
}

// getEnvironStorage creates an Environ from the config in state and
// returns its storage interface.
func getEnvironStorage(st *state.State) (storage.Storage, error) {
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot get environment config: %v", err)
	}
	env, err := environs.New(envConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot access environment: %v", err)
	}
	return env.Storage(), nil
}

func (h *charmsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.authenticate(w, r); err != nil {
		h.authError(w)
		return
	}

	switch r.Method {
	case "POST":
		h.processPost(w, r)
	// Possible future extensions, like GET.
	default:
		h.sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", r.Method))
	}
}
