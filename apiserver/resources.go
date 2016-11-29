// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/state"
)

// XXX needs tests

// resourceUploadHandler handles resources uploads for model migrations.
type resourceUploadHandler struct {
	ctxt          httpContext
	stateAuthFunc func(*http.Request) (*state.State, error)
}

func (h *resourceUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Validate before authenticate because the authentication is dependent
	// on the state connection that is determined during the validation.
	st, err := h.stateAuthFunc(r)
	if err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	defer h.ctxt.release(st)

	switch r.Method {
	case "POST":
		if err := h.processPost(r, st); err != nil {
			if err := sendError(w, err); err != nil {
				logger.Errorf("%v", err)
			}
			return
		}
		// XXX this should send a JSON result (resource/api.CharmResource)
		w.WriteHeader(http.StatusOK)
	default:
		if err := sendError(w, errors.MethodNotAllowedf("unsupported method: %q", r.Method)); err != nil {
			logger.Errorf("%v", err)
		}
	}
}

// processPost handles a tools upload POST request after authentication.
func (h *resourceUploadHandler) processPost(r *http.Request, st *state.State) error {
	query := r.URL.Query()

	applicationID := query.Get("application")
	if applicationID == "" {
		return errors.NotValidf("missing application")
	}
	userID := query.Get("user")
	if userID == "" {
		return errors.NotValidf("missing user")
	}
	res, err := queryToResource(query)
	if err != nil {
		return errors.Trace(err)
	}
	rSt, err := st.Resources()
	if err != nil {
		return errors.Trace(err)
	}
	_, err = rSt.SetResource(applicationID, userID, res, r.Body)
	return errors.Annotate(err, "resource upload failed")
}

func queryToResource(query url.Values) (charmresource.Resource, error) {
	var err error
	empty := charmresource.Resource{}

	res := charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        query.Get("name"),
			Path:        query.Get("path"),
			Description: query.Get("description"),
		},
	}
	if res.Name == "" {
		return empty, errors.NotValidf("missing name")
	}
	if res.Path == "" {
		return empty, errors.NotValidf("missing path")
	}
	if res.Description == "" {
		return empty, errors.NotValidf("missing description")
	}
	res.Type, err = charmresource.ParseType(query.Get("type"))
	if err != nil {
		return empty, errors.NotValidf("type")
	}
	res.Origin, err = charmresource.ParseOrigin(query.Get("origin"))
	if err != nil {
		return empty, errors.NotValidf("origin")
	}
	res.Revision, err = strconv.Atoi(query.Get("revision"))
	if err != nil {
		return empty, errors.NotValidf("revision")
	}
	res.Size, err = strconv.ParseInt(query.Get("size"), 10, 64)
	if err != nil {
		return empty, errors.Trace(err)
	}
	res.Fingerprint, err = charmresource.ParseFingerprint(query.Get("fingerprint"))
	if err != nil {
		return empty, errors.Annotate(err, "invalid fingerprint")
	}
	return res, nil
}
