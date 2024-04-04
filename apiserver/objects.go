// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type objectsCharmHTTPHandler struct {
	GetHandler          FailableHandlerFunc
	PutHandler          FailableHandlerFunc
	LegacyCharmsHandler http.Handler
}

func (h *objectsCharmHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "GET":
		err = errors.Annotate(h.GetHandler(w, r), "cannot retrieve charm")
		if err == nil {
			// Chain call to legacy (REST API) charms handler
			h.LegacyCharmsHandler.ServeHTTP(w, r)
		}
	case "PUT":
		err = errors.Annotate(h.PutHandler(w, r), "cannot upload charm")
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

// objectsCharmHandler handles charm upload through S3-compatible HTTPS in the
// API server.
type objectsCharmHandler struct {
	ctxt          httpContext
	stateAuthFunc func(*http.Request) (*state.PooledState, error)
}

func (h *objectsCharmHandler) ServeUnsupported(w http.ResponseWriter, r *http.Request) error {
	return errors.Trace(emitUnsupportedMethodErr(r.Method))
}

// ServeGet serves the GET method for the S3 API. This is the equivalent of the
// `GetObject` method in the AWS S3 API.
// Since juju's objects (S3) API only acts as a shim, this method will only
// rewrite the http request for it to be correctly processed by the legacy
// '/charms' handler.
func (h *objectsCharmHandler) ServeGet(w http.ResponseWriter, r *http.Request) error {
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

	query.Add("url", ch.URL())
	query.Add("file", "*")
	r.URL.RawQuery = query.Encode()

	return nil
}

// ServePut serves the PUT method for the S3 API. This is the equivalent of the
// `PutObject` method in the AWS S3 API.
// Since juju's objects (S3) API only acts as a shim, this method will only
// rewrite the http request for it to be correctly processed by the legacy
// '/charms' handler.
func (h *objectsCharmHandler) ServePut(w http.ResponseWriter, r *http.Request) error {
	// Make sure the content type is zip.
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/zip" {
		return errors.BadRequestf("expected Content-Type: application/zip, got: %v", contentType)
	}

	st, err := h.stateAuthFunc(r)
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Release()

	// Add a charm to the store provider.
	charmURL, err := h.processPut(r, st.State)
	if err != nil {
		return errors.NewBadRequest(err, "")
	}
	return errors.Trace(sendStatusAndHeadersAndJSON(w, http.StatusOK, map[string]string{"Juju-Curl": charmURL.String()}, &params.CharmsResponse{CharmURL: charmURL.String()}))
}

func (h *objectsCharmHandler) processPut(r *http.Request, st *state.State) (*charm.URL, error) {
	query := r.URL.Query()
	name, shaFromQuery, err := splitNameAndSHAFromQuery(query)
	if err != nil {
		return nil, errors.Trace(err)
	}

	curlStr := r.Header.Get("Juju-Curl")
	curl, err := charm.ParseURL(curlStr)
	if err != nil {
		return nil, errors.BadRequestf("%q is not a valid charm url", curlStr)
	}
	curl.Name = name

	schema := curl.Schema
	if schema != "local" {
		// charmhub charms may only be uploaded into models
		// which are being imported during model migrations.
		// There's currently no other time where it makes sense
		// to accept repository charms through this endpoint.
		if isImporting, err := modelIsImporting(st); err != nil {
			return nil, errors.Trace(err)
		} else if !isImporting {
			return nil, errors.New("non-local charms may only be uploaded during model migration import")
		}
	}

	charmFileName, err := writeCharmToTempFile(r.Body)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer os.Remove(charmFileName)

	charmSHA, _, err := utils.ReadFileSHA256(charmFileName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// ReadFileSHA256 returns a full 64 char SHA256. However, charm refs
	// only use the first 7 chars. So truncate the sha to match
	charmSHA = charmSHA[0:7]
	if charmSHA != shaFromQuery {
		return nil, errors.BadRequestf("Uploaded charm sha256 (%v) does not match sha in url (%v)", charmSHA, shaFromQuery)
	}

	archive, err := charm.ReadCharmArchive(charmFileName)
	if err != nil {
		return nil, errors.BadRequestf("invalid charm archive: %v", err)
	}

	if curl.Revision == -1 {
		curl.Revision = archive.Revision()
	}

	switch charm.Schema(schema) {
	case charm.Local:
		curl, err = st.PrepareLocalCharmUpload(curl.String())
		if err != nil {
			return nil, errors.Trace(err)
		}

	case charm.CharmHub:
		if _, err := st.PrepareCharmUpload(curl.String()); err != nil {
			return nil, errors.Trace(err)
		}

	default:
		return nil, errors.Errorf("unsupported schema %q", schema)
	}

	return curl, errors.Trace(RepackageAndUploadCharm(st, archive, curl))
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
