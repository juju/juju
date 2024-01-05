// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/core/objectstore"
)

// ObjectStoreGetter is an interface for getting an object store.
type ObjectStoreGetter interface {
	// GetObjectStore returns the object store for the given namespace.
	GetObjectStore(context.Context, string) (objectstore.ObjectStore, error)
}

type objectsCharmHTTPHandler struct {
	ctxt              httpContext
	objectStoreGetter ObjectStoreGetter
}

func (h *objectsCharmHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "GET":
		err = errors.Annotate(h.ServeGet(w, r), "cannot retrieve charm")
	default:
		http.Error(w, fmt.Sprintf("http method %s not implemented", r.Method), http.StatusNotImplemented)
		return
	}

	if err != nil {
		if err := sendJSONError(w, r, errors.Trace(err)); err != nil {
			logger.Errorf("%v", errors.Annotate(err, "cannot return error to user"))
		}
	}
}

// ServeGet serves the GET method for the S3 API. This is the equivalent of the
// `GetObject` method in the AWS S3 API.
func (h *objectsCharmHTTPHandler) ServeGet(w http.ResponseWriter, r *http.Request) error {
	st, _, err := h.ctxt.stateForRequestAuthenticated(r)
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Release()

	query := r.URL.Query()
	charmObjectID := query.Get(":object")

	// Path param is {charmName}-{charmSha256[0:7]} so we need to split it.
	charmSplit := strings.Split(charmObjectID, "-")
	if len(charmSplit) < 2 {
		return errors.NewBadRequest(errors.New(fmt.Sprintf("wrong charms object path %q", charmObjectID)), "")
	}
	charmSha256 := charmSplit[len(charmSplit)-1]

	// Retrieve charm from state.
	ch, err := st.CharmFromSha256(charmSha256)
	if err != nil {
		return errors.Annotate(err, "cannot get charm from state")
	}

	// Check if the charm is still pending to be downloaded and return back
	// a suitable error.
	if !ch.IsUploaded() {
		return errors.NewNotYetAvailable(nil, ch.URL())
	}

	// Get the underlying object store for the model UUID, which we can then
	// retrieve the blob from.
	store, err := h.objectStoreGetter.GetObjectStore(r.Context(), st.ModelUUID())
	if err != nil {
		return errors.Annotate(err, "cannot get object store")
	}

	// Use the storage to retrieve the charm archive.
	reader, _, err := store.Get(r.Context(), ch.StoragePath())
	if err != nil {
		return errors.Annotate(err, "cannot get charm from model storage")
	}
	defer reader.Close()

	_, err = io.Copy(w, reader)
	if err != nil {
		return errors.Annotate(err, "error processing charm archive download")
	}

	return nil
}
