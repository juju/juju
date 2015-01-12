// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	apihttp "github.com/juju/juju/apiserver/http"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/container"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/imagestorage"
)

// imagesDownloadHandler handles image download through HTTPS in the API server.
type imagesDownloadHandler struct {
	httpHandler
}

func (h *imagesDownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h.validateEnvironUUID(r); err != nil {
		h.sendError(w, http.StatusNotFound, err.Error())
		return
	}
	switch r.Method {
	case "GET":
		err := h.processGet(r, w)
		if err != nil {
			logger.Errorf("GET(%s) failed: %v", r.URL, err)
			h.sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
	default:
		h.sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %q", r.Method))
	}
}

// sendJSON sends a JSON-encoded response to the client.
func (h *imagesDownloadHandler) sendJSON(w http.ResponseWriter, statusCode int, response *params.ErrorResult) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	w.Write(body)
	return nil
}

// sendError sends a JSON-encoded error response.
func (h *imagesDownloadHandler) sendError(w http.ResponseWriter, statusCode int, message string) {
	logger.Debugf("sending error: %v %v", statusCode, message)
	err := common.ServerError(errors.New(message))
	if err := h.sendJSON(w, statusCode, &params.ErrorResult{Error: err}); err != nil {
		logger.Errorf("failed to send error: %v", err)
	}
}

// processGet handles an image GET request.
func (h *imagesDownloadHandler) processGet(r *http.Request, resp http.ResponseWriter) error {
	// Get the parameters from the query.
	kind := r.URL.Query().Get(":kind")
	series := r.URL.Query().Get(":series")
	arch := r.URL.Query().Get(":arch")
	envuuid := r.URL.Query().Get(":envuuid")

	// Get the image details from storage.
	storage := h.state.ImageStorage()
	metadata, imageReader, err := storage.Image(kind, series, arch)
	// Not in storage, so go fetch it.
	if errors.IsNotFound(err) {
		metadata, imageReader, err = h.fetchAndCacheLxcImage(storage, envuuid, series, arch)
		if err != nil {
			return errors.Annotate(err, "error fetching and caching image")
		}
	}
	if err != nil {
		return errors.Annotate(err, "error getting image from storage")
	}
	defer imageReader.Close()

	// Stream the image to the caller.
	logger.Debugf("streaming image from state blobstore: %+v", metadata)
	resp.Header().Set("Content-Type", "application/x-tar-gz")
	resp.Header().Set("Digest", fmt.Sprintf("%s=%s", apihttp.DigestSHA, metadata.SHA256))
	resp.Header().Set("Content-Length", fmt.Sprint(metadata.Size))
	resp.WriteHeader(http.StatusOK)
	if _, err := io.Copy(resp, imageReader); err != nil {
		return errors.Annotate(err, "while streaming image")
	}
	return nil
}

// fetchAndCacheLxcImage fetches an lxc image tarball from http://cloud-images.ubuntu.com
// and caches it in the state blobstore.
func (h *imagesDownloadHandler) fetchAndCacheLxcImage(storage imagestorage.Storage, envuuid, series, arch string) (
	*imagestorage.Metadata, io.ReadCloser, error,
) {
	imageURL, err := container.ImageDownloadURL(instance.LXC, series, arch)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "cannot determine LXC image URL: %v", err)
	}

	// Fetch the image checksum.
	imageFilename := path.Base(imageURL)
	shafile := strings.Replace(imageURL, imageFilename, "SHA256SUMS", -1)
	shaResp, err := http.Get(shafile)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "cannot get sha256 data from %v", shafile)
	}
	defer shaResp.Body.Close()
	shaInfo, err := ioutil.ReadAll(shaResp.Body)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "cannot read sha256 data from %v", shafile)
	}

	// The sha file has lines like:
	// "<checksum> *<imageFilename>"
	checksum := ""
	for _, line := range strings.Split(string(shaInfo), "\n") {
		parts := strings.Split(line, "*")
		if len(parts) != 2 {
			continue
		}
		if parts[1] == imageFilename {
			checksum = strings.TrimSpace(parts[0])
			break
		}
	}
	if checksum == "" {
		return nil, nil, errors.Errorf("cannot find sha256 checksum for %v", imageFilename)
	}

	// Fetch the image.
	logger.Debugf("fetching LXC image from: %v", imageURL)
	resp, err := http.Get(imageURL)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "cannot get image from %v", imageURL)
	}
	logger.Debugf("lxc image has size: %v bytes", resp.ContentLength)
	defer resp.Body.Close()

	hash := sha256.New()
	// Set up a chain of readers to pull in the data and calculate the checksum.
	rdr := io.TeeReader(resp.Body, hash)

	metadata := &imagestorage.Metadata{
		EnvUUID:   envuuid,
		Kind:      string(instance.LXC),
		Series:    series,
		Arch:      arch,
		Size:      resp.ContentLength,
		SHA256:    checksum,
		SourceURL: imageURL,
	}

	// Stream the image to storage.
	err = storage.AddImage(rdr, metadata)
	if err != nil {
		return nil, nil, err
	}
	// Better check the downloaded image checksum.
	downloadChecksum := fmt.Sprintf("%x", hash.Sum(nil))
	if downloadChecksum != checksum {
		if err := storage.DeleteImage(metadata); err != nil {
			logger.Errorf("checksum mismatch, failed to delete image from storage: %v", err)
		}
		return nil, nil, errors.Errorf("download checksum mismatch %s != %s", downloadChecksum, checksum)
	}

	return storage.Image(string(instance.LXC), series, arch)
}
