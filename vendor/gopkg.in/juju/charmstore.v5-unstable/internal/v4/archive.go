// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v4 // import "gopkg.in/juju/charmstore.v5-unstable/internal/v4"

import (
	"net/http"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"

	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
	"gopkg.in/juju/charmstore.v5-unstable/internal/v5"
)

// serveArchive returns a handler for /archive that falls back to v5ServeArchive
// for all operations not handled by v4.
func (h ReqHandler) serveArchive(v5ServeArchive router.IdHandler) router.IdHandler {
	get := h.ResolvedIdHandler(h.serveGetArchive)
	return func(id *charm.URL, w http.ResponseWriter, req *http.Request) error {
		if req.Method == "GET" {
			return get(id, w, req)
		}
		return v5ServeArchive(id, w, req)
	}
}

func (h ReqHandler) serveGetArchive(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	if err := h.AuthorizeEntityForOp(id, req, v5.OpReadWithTerms); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	blob, err := h.Store.OpenBlobPreV5(id)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	defer blob.Close()
	h.SendEntityArchive(id, w, req, blob)
	return nil
}

// GET id/archive/path
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idarchivepath
func (h ReqHandler) serveArchiveFile(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	blob, err := h.Store.OpenBlobPreV5(id)
	if err != nil {
		return errgo.Notef(err, "cannot open archive data for %v", id)
	}
	defer blob.Close()
	return h.ServeBlobFile(w, req, id, blob)
}
