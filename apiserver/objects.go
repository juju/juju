// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/charm"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/core/objectstore"
)

// endpointMethodHandlerFunc desribes the signature for our functions which handle
// requests made to a specific endpoint, with a specific method
type endpointMethodHandlerFunc func(http.ResponseWriter, *http.Request) error

// ObjectStoreGetter is an interface for getting an object store.
type ObjectStoreGetter interface {
	// GetObjectStore returns the object store for the given namespace.
	GetObjectStore(context.Context, string) (objectstore.ObjectStore, error)
}

type objectsCharmHTTPHandler struct {
	ctxt              httpContext
	objectStoreGetter ObjectStoreGetter
	LegacyPostHandler endpointMethodHandlerFunc
}

func (h *objectsCharmHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "GET":
		err = errors.Annotate(h.ServeGet(w, r), "cannot retrieve charm")
	case "PUT":
		err = errors.Annotate(h.ServePut(w, r), "cannot upload charm")
		if err == nil {
			// Chain call to legacy (REST API) charms handler
			err = h.LegacyPostHandler(w, r)
		}
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

	_, charmSha256, err := splitNameAndSHAFromQuery(query)
	if err != nil {
		return err
	}

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

// ServePut serves the PUT method for the S3 API. This is the equivalent of the
// `PutObject` method in the AWS S3 API.
// Since juju's objects (S3) API only acts as a shim, this method will only
// rewrite the http request for it to be correctly processed by the legacy
// '/charms' handler.
//
// TODO(jack-w-shaw) Implement properly i.e. no longer shim around the legacy handler
func (h *objectsCharmHTTPHandler) ServePut(w http.ResponseWriter, r *http.Request) error {
	// Make sure the content type is zip.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/zip" {
		return errors.BadRequestf("expected Content-Type: application/zip, got: %v", contentType)
	}

	query := r.URL.Query()
	name, shaFromQuery, err := splitNameAndSHAFromQuery(query)
	if err != nil {
		return err
	}

	charmFileName, err := writeCharmToTempFile(r.Body)
	if err != nil {
		return errors.Trace(err)
	}
	defer os.Remove(charmFileName)

	curlStr := r.Header.Get("Juju-Curl")
	curl, err := charm.ParseURL(curlStr)
	if err != nil {
		return errors.BadRequestf("%q is not a valid charm url", curlStr)
	}
	curl.Name = name

	charmSHA, _, err := utils.ReadFileSHA256(charmFileName)
	if err != nil {
		return errors.Trace(err)
	}
	// ReadFileSHA256 returns a full 64 char SHA256. However, charm refs
	// only use the first 7 chars. So truncate the sha to match
	charmSHA = charmSHA[0:7]

	if charmSHA != shaFromQuery {
		return errors.BadRequestf("Uploaded charm sha256 (%v) does not match sha in url (%v)", charmSHA, shaFromQuery)
	}

	query.Add("schema", curl.Schema)
	query.Add("name", curl.Name)
	query.Add("revision", strconv.Itoa(curl.Revision))
	query.Add("series", curl.Series)
	query.Add("arch", curl.Architecture)
	r.URL.RawQuery = query.Encode()

	// We have already read the request body, so we need to refresh it
	// so it can be read again in future
	r.Body, err = os.Open(charmFileName)
	if err != nil {
		return errors.Trace(err)
	}

	// The legacy charm uplaod handler expects a POST request
	r.Method = "POST"

	return nil
}

func splitNameAndSHAFromQuery(query url.Values) (string, string, error) {
	charmObjectID := query.Get(":object")

	// Path param is {charmName}-{charmSha256[0:7]} so we need to split it.
	// NOTE: charmName can contain "-", so we cannot simply strings.Split
	splitIndex := strings.LastIndex(charmObjectID, "-")
	if splitIndex == -1 {
		return "", "", errors.BadRequestf("%q is not a valid charm object path", charmObjectID)
	}
	name, sha := charmObjectID[:splitIndex], charmObjectID[splitIndex+1:]
	return name, sha, nil
}
