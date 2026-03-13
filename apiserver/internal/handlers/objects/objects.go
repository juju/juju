// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objects

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"

	jujuerrors "github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	internalhttp "github.com/juju/juju/apiserver/internal/http"
	"github.com/juju/juju/core/objectstore"
	domainobjectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
)

// ObjectStoreService is an interface that provides a method to get an object
// from an object store.
type ObjectStoreService interface {
	// GetBySHA256 returns a reader for the object with the given SHA256 hash.
	GetBySHA256(ctx context.Context, sha256 string) (io.ReadCloser, objectstore.Digest, error)
}

// ObjectStoreServiceGetter is an interface that provides a method to get an
// object store service.
type ObjectStoreServiceGetter interface {
	ObjectStore(*http.Request) (ObjectStoreService, error)
}

// ObjectsHTTPHandler implements the http.Handler interface for the objects API.
type ObjectsHTTPHandler struct {
	objectStoreGetter ObjectStoreServiceGetter
}

// NewObjectsHTTPHandler returns a new ObjectsHTTPHandler.
func NewObjectsHTTPHandler(
	objectStoreGetter ObjectStoreServiceGetter,
) *ObjectsHTTPHandler {
	return &ObjectsHTTPHandler{
		objectStoreGetter: objectStoreGetter,
	}
}

// ServeHTTP implements the http.Handler interface.
func (h *ObjectsHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "GET":
		err = h.ServeGet(w, r)
		if err != nil {
			err = errors.Errorf("cannot retrieve object: %w", err)
		}
	default:
		http.Error(w, fmt.Sprintf("http method %s not implemented", r.Method), http.StatusNotImplemented)
		return
	}

	if err == nil {
		return
	}

	requestID := r.Header.Get("x-amz-request-id")
	hostID := r.Header.Get("x-amz-id-2")

	if err := sendS3JSONError(w, requestID, hostID, err); err != nil {
		logger.Errorf(r.Context(), "%v", errors.Errorf("cannot return error to user: %w", err))
	}
}

// ServeGet serves the GET method for the S3 API. This is the equivalent of the
// `GetObject` method in the AWS S3 API.
func (h *ObjectsHTTPHandler) ServeGet(w http.ResponseWriter, r *http.Request) error {
	objectStore, err := h.objectStoreGetter.ObjectStore(r)
	if err != nil {
		return errors.Capture(err)
	}

	query := r.URL.Query()
	sha256 := query.Get(":object")
	if sha256 == "" {
		return jujuerrors.BadRequestf("missing object sha256")
	}

	reader, digest, err := objectStore.GetBySHA256(r.Context(), sha256)
	if errors.IsOneOf(err, domainobjectstoreerrors.ErrInvalidHashLength, domainobjectstoreerrors.ErrInvalidHash) {
		return jujuerrors.BadRequestf("invalid object sha256: %s", sha256)
	} else if errors.Is(err, objectstoreerrors.ObjectNotFound) {
		return jujuerrors.NotFoundf("object: %s", sha256)
	} else if err != nil {
		return errors.Capture(err)
	}
	defer reader.Close()

	// Set the content-length before the copy, so the client knows how much to
	// expect.
	w.Header().Set("Content-Length", strconv.FormatInt(digest.Size, 10))

	w.Header().Set("x-amzn-requestid", r.Header.Get("x-amz-request-id"))
	w.Header().Set("x-amzn-id-2", r.Header.Get("x-amz-id-2"))

	// We want to send back the checksum header to ensure nothing got corrupted
	// in transit. Objects are content addressable, we can guarantee that the
	// object found for the given hash is the same. So we just need to encode
	// the hash back for the s3 client to verify it.
	decodedHex, err := hex.DecodeString(sha256)
	if err != nil {
		return errors.Capture(err)
	}
	w.Header().Set("x-amz-checksum-sha256", base64.StdEncoding.EncodeToString(decodedHex))

	size, err := io.Copy(w, reader)
	if err != nil {
		return errors.Errorf("processing object download: %w", err)
	}

	// There isn't much we can do if the size doesn't match, but we can log it.
	if digest.Size != size {
		logger.Warningf(r.Context(), "expected size %d, got %d when reading %v", digest.Size, size, sha256)
	}

	return nil
}

// S3Error represents the structure of an error response from the S3 API.
// If we ever support XML, this would need to be updated to include XML tags.
type S3Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
	HostID    string `json:"hostId"`
}

// sendJSONError sends a JSON-encoded error response.  Note the
// difference from the error response sent by the sendError function -
// the error is encoded in the Error field as a string, not an Error
// object.
func sendS3JSONError(w http.ResponseWriter, requestID, hostID string, err error) error {
	perr, status := apiservererrors.ServerErrorAndStatus(err)

	code := "InternalError"
	switch status {
	case http.StatusBadRequest:
		code = "InvalidRequest"
	case http.StatusForbidden:
		code = "InvalidAccessKeyId"
	case http.StatusNotFound:
		code = "NoSuchKey"
	}

	return errors.Capture(internalhttp.SendStatusAndJSON(w, status, S3Error{
		Code:      code,
		Message:   perr.Message,
		RequestID: requestID,
		HostID:    hostID,
	}))
}
