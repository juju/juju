// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objects

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"

	jujuerrors "github.com/juju/errors"

	objectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	"github.com/juju/juju/internal/errors"
)

// ObjectStoreService is an interface that provides a method to get an object
// from an object store.
type ObjectStoreService interface {
	// GetBySHA256 returns a reader for the object with the given SHA256 hash.
	GetBySHA256(ctx context.Context, sha256 string) (io.ReadCloser, int64, error)
}

// ObjectStoreServiceGetter is an interface that provides a method to get an
// object store service.
type ObjectStoreServiceGetter interface {
	ObjectStore(*http.Request) (ObjectStoreService, error)
}

// ObjectsHTTPHandler implements the http.Handler interface for the objects API.
type ObjectsHTTPHandler struct {
	stateGetter       StateGetter
	objectStoreGetter ObjectStoreServiceGetter
}

// NewObjectsHTTPHandler returns a new ObjectsHTTPHandler.
func NewObjectsHTTPHandler(
	stateGetter StateGetter,
	objectStoreGetter ObjectStoreServiceGetter,
) *ObjectsHTTPHandler {
	return &ObjectsHTTPHandler{
		stateGetter:       stateGetter,
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

	if err := sendJSONError(w, errors.Capture(err)); err != nil {
		logger.Errorf(r.Context(), "%v", errors.Errorf("cannot return error to user: %w", err))
	}
}

// ServeGet serves the GET method for the S3 API. This is the equivalent of the
// `GetObject` method in the AWS S3 API.
func (h *ObjectsHTTPHandler) ServeGet(w http.ResponseWriter, r *http.Request) error {
	service, err := h.objectStoreGetter.ObjectStore(r)
	if err != nil {
		return errors.Capture(err)
	}

	query := r.URL.Query()
	sha256 := query.Get(":object")
	if sha256 == "" {
		return jujuerrors.BadRequestf("missing object sha256")
	}

	reader, readerSize, err := service.GetBySHA256(r.Context(), sha256)
	if errors.Is(err, objectstoreerrors.ErrNotFound) {
		return jujuerrors.NotFoundf("object: %s", sha256)
	} else if err != nil {
		return errors.Capture(err)
	}
	defer reader.Close()

	// Set the content-length before the copy, so the client knows how much to
	// expect.
	w.Header().Set("Content-Length", strconv.FormatInt(readerSize, 10))

	size, err := io.Copy(w, reader)
	if err != nil {
		return errors.Errorf("processing object download: %w", err)
	}

	// There isn't much we can do if the size doesn't match, but we can log it.
	if readerSize != size {
		logger.Warningf(r.Context(), "expected size %d, got %d when reading %v", readerSize, size, sha256)
	}

	return nil
}
