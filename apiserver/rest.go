// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/storage"
)

// RestHTTPHandler creates is a http.Handler which serves ReST requests.
type RestHTTPHandler struct {
	GetHandler FailableHandlerFunc
}

// ServeHTTP is defined on handler.Handler.
func (h *RestHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var err error
	switch r.Method {
	case "GET":
		err = errors.Annotate(h.GetHandler(w, r), "cannot retrieve model data")
	default:
		err = emitUnsupportedMethodErr(r.Method)
	}

	if err != nil {
		if err := sendJSONError(w, r, errors.Trace(err)); err != nil {
			logger.Errorf("%v", errors.Annotate(err, "cannot return error to user"))
		}
	}
}

// modelRestHandler handles ReST requests through HTTPS in the API server.
type modelRestHandler struct {
	ctxt    httpContext
	dataDir string
}

// ServeGet handles http GET requests.
func (h *modelRestHandler) ServeGet(w http.ResponseWriter, r *http.Request) error {
	if r.Method != "GET" {
		return errors.Trace(emitUnsupportedMethodErr(r.Method))
	}

	st, _, err := h.ctxt.stateForRequestAuthenticated(r)
	if err != nil {
		return errors.Trace(err)
	}
	defer st.Release()

	return errors.Trace(h.processGet(r, w, st.State))
}

// processGet handles a ReST GET request after authentication.
func (h *modelRestHandler) processGet(r *http.Request, w http.ResponseWriter, st *state.State) error {
	query := r.URL.Query()
	entity := query.Get(":entity")
	// TODO(wallyworld) - support more than just "remote-application"
	switch entity {
	case "remote-application":
		return h.processRemoteApplication(r, w, st)
	default:
		return errors.NotSupportedf("entity %v", entity)
	}
}

// processRemoteApplication handles a request for attributes on remote applications.
func (h *modelRestHandler) processRemoteApplication(r *http.Request, w http.ResponseWriter, st *state.State) error {
	query := r.URL.Query()
	name := query.Get(":name")
	remoteApp, err := st.RemoteApplication(name)
	if err != nil {
		return errors.Trace(err)
	}
	attribute := query.Get(":attribute")
	// TODO(wallyworld) - support more than just "icon"
	if attribute != "icon" {
		return errors.NotSupportedf("attribute %v on entity %v", attribute, name)
	}

	// Get the backend state for the source model so we can lookup the app in that model to get the charm details.
	offerUUID := remoteApp.OfferUUID()
	if offerUUID == "" {
		return h.byteSender(w, ".svg", []byte(common.DefaultCharmIcon))
	}
	sourceModelUUID := remoteApp.SourceModel().Id()
	sourceSt, err := h.ctxt.srv.shared.statePool.Get(sourceModelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	defer sourceSt.Release()

	offers := state.NewApplicationOffers(sourceSt.State)
	offer, err := offers.ApplicationOfferForUUID(offerUUID)
	if err != nil {
		return errors.Trace(err)
	}
	app, err := sourceSt.Application(offer.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	ch, _, err := app.Charm()
	if err != nil {
		return errors.Trace(err)
	}

	store := storage.NewStorage(sourceSt.ModelUUID(), sourceSt.MongoSession())
	// Use the storage to retrieve and save the charm archive.
	charmPath, err := common.ReadCharmFromStorage(store, h.dataDir, ch.StoragePath())
	if errors.IsNotFound(err) {
		return h.byteSender(w, ".svg", []byte(common.DefaultCharmIcon))
	}
	if err != nil {
		return errors.Trace(err)
	}
	iconContents, err := common.CharmArchiveEntry(charmPath, "icon.svg", true)
	if errors.IsNotFound(err) {
		return h.byteSender(w, ".svg", []byte(common.DefaultCharmIcon))
	}
	if err != nil {
		return errors.Trace(err)
	}
	return h.byteSender(w, ".svg", iconContents)
}

func (h *modelRestHandler) byteSender(w http.ResponseWriter, ext string, contents []byte) error {
	ctype := mime.TypeByExtension(ext)
	if ctype != "" {
		// Older mime.types may map .js to x-javascript.
		// Map it to javascript for consistency.
		if ctype == params.ContentTypeXJS {
			ctype = params.ContentTypeJS
		}
		w.Header().Set("Content-Type", ctype)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(contents)))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, bytes.NewReader(contents))
	return nil
}
