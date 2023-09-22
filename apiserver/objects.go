// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
)

type objectsCharmHTTPHandler struct {
	GetHandler          FailableHandlerFunc
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
	ctxt httpContext
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

	query.Add("url", ch.URL())
	query.Add("file", "*")
	r.URL.RawQuery = query.Encode()

	return nil
}
