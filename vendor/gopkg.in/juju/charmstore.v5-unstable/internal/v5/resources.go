// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v5 // import "gopkg.in/juju/charmstore.v5-unstable/internal/v5"

import (
	"encoding/hex"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/juju/httprequest"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"

	"gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
)

// POST id/resource/name
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#post-idresourcesname
//
// GET  id/resource/name[/revision]
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idresourcesnamerevision
func (h *ReqHandler) serveResources(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	// Resources are "published" using "POST id/publish" so we don't
	// support PUT here.
	// TODO(ericsnow) Support DELETE to remove a resource (like serveArchive)?
	switch req.Method {
	case "GET":
		return h.serveDownloadResource(id, w, req)
	case "POST":
		return h.serveUploadResource(id, w, req)
	default:
		return errgo.WithCausef(nil, params.ErrMethodNotAllowed, "%s not allowed", req.Method)
	}
}

func (h *ReqHandler) serveDownloadResource(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	rid, err := parseResourceId(strings.TrimPrefix(req.URL.Path, "/"))
	if err != nil {
		return errgo.WithCausef(err, params.ErrNotFound, "")
	}
	ch, err := h.entityChannel(id)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	r, err := h.Store.ResolveResource(id, rid.Name, rid.Revision, ch)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	blob, err := h.Store.OpenResourceBlob(r)
	if err != nil {
		return errgo.Notef(err, "cannot open resource blob")
	}
	defer blob.Close()
	header := w.Header()
	setArchiveCacheControl(w.Header(), h.isPublic(id))
	header.Set(params.ContentHashHeader, blob.Hash)

	// TODO(rog) should we set connection=close here?
	// See https://codereview.appspot.com/5958045
	serveContent(w, req, blob.Size, blob)
	return nil
}

func (h *ReqHandler) serveUploadResource(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	if id.URL.Series == "bundle" {
		return errgo.WithCausef(nil, params.ErrForbidden, "cannot upload a resource to a bundle")
	}
	name := strings.TrimPrefix(req.URL.Path, "/")
	if !validResourceName(name) {
		return badRequestf(nil, "invalid resource name")
	}
	hash := req.Form.Get("hash")
	if hash == "" {
		return badRequestf(nil, "hash parameter not specified")
	}
	if req.ContentLength == -1 {
		return badRequestf(nil, "Content-Length not specified")
	}
	e, err := h.Cache.Entity(&id.URL, charmstore.FieldSelector("charmmeta"))
	if err != nil {
		// Should never happen, as the entity will have been cached
		// when the charm URL was resolved.
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	r, ok := e.CharmMeta.Resources[name]
	if !ok {
		return errgo.WithCausef(nil, params.ErrForbidden, "resource %q not found in charm metadata", name)
	}
	if r.Type != resource.TypeFile {
		return errgo.WithCausef(nil, params.ErrForbidden, "non-file resource types not supported")
	}
	if filename := req.Form.Get("filename"); filename != "" {
		if charmExt := path.Ext(r.Path); charmExt != "" {
			// The resource has a filename extension. Check that it matches.
			if charmExt != path.Ext(filename) {
				return errgo.WithCausef(nil, params.ErrForbidden, "filename extension mismatch (got %q want %q)", path.Ext(filename), charmExt)
			}
		}
	}
	rdoc, err := h.Store.UploadResource(id, name, req.Body, hash, req.ContentLength)
	if err != nil {
		return errgo.Mask(err)
	}
	return httprequest.WriteJSON(w, http.StatusOK, &params.ResourceUploadResponse{
		Revision: rdoc.Revision,
	})
}

// GET id/meta/resource
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaresources
func (h *ReqHandler) metaResources(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	if entity.URL.Series == "bundle" {
		return nil, nil
	}
	ch, err := h.entityChannel(id)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	resources, err := h.Store.ListResources(id, ch)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	results := make([]params.Resource, len(resources))
	for i, res := range resources {
		result, err := fromResourceDoc(res, entity.CharmMeta.Resources)
		if err != nil {
			return nil, err
		}
		results[i] = *result
	}
	return results, nil
}

// GET id/meta/resource/*name*[/*revision]
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaresourcesnamerevision
func (h *ReqHandler) metaResourcesSingle(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	data, err := h.metaResourcesSingle0(entity, id, path, flags, req)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			logger.Infof("replacing not-found error on %s/meta/resources%s: %v (%#v)", id.URL.Path(), path, err, err)
			// It's a not-found error; return nothing
			// so that it's OK to use this in a bulk meta request.
			return nil, nil
		}
		return nil, errgo.Mask(err)
	}
	return data, nil
}

func (h *ReqHandler) metaResourcesSingle0(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	if id.URL.Series == "bundle" {
		return nil, nil
	}
	rid, err := parseResourceId(strings.TrimPrefix(path, "/"))
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrNotFound, "")
	}
	ch, err := h.entityChannel(id)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	doc, err := h.Store.ResolveResource(id, rid.Name, rid.Revision, ch)
	if err != nil {
		if errgo.Cause(err) != params.ErrNotFound || rid.Revision != -1 {
			return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		// The resource wasn't found and we're not asking for a specific
		// revision. If the resource actually exists in the charm metadata,
		// return a placeholder document as would be returned by
		// the /meta/resources (ListResources) endpoint.
		if _, ok := entity.CharmMeta.Resources[rid.Name]; !ok {
			return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		doc = &mongodoc.Resource{
			Name:     rid.Name,
			Revision: -1,
		}
	}
	result, err := fromResourceDoc(doc, entity.CharmMeta.Resources)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	return result, nil
}

func fromResourceDoc(doc *mongodoc.Resource, resources map[string]resource.Meta) (*params.Resource, error) {
	meta, ok := resources[doc.Name]
	if !ok {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "resource %q not found in charm", doc.Name)
	}
	r := &params.Resource{
		Name:        doc.Name,
		Revision:    -1,
		Type:        meta.Type.String(),
		Path:        meta.Path,
		Description: meta.Description,
	}
	if doc.BlobHash == "" {
		// No hash implies that there is no file (the entry
		// is just a placeholder), so we don't fill in
		// blob details.
		return r, nil
	}
	rawHash, err := hex.DecodeString(doc.BlobHash)
	if err != nil {
		return nil, errgo.Notef(err, "cannot decode blob hash %q", doc.BlobHash)
	}
	r.Size = doc.Size
	r.Fingerprint = rawHash
	r.Revision = doc.Revision
	return r, nil
}

func parseResourceId(path string) (mongodoc.ResourceRevision, error) {
	i := strings.Index(path, "/")
	if i == -1 {
		return mongodoc.ResourceRevision{
			Name:     path,
			Revision: -1,
		}, nil
	}
	revno, err := strconv.Atoi(path[i+1:])
	if err != nil {
		return mongodoc.ResourceRevision{}, errgo.Newf("malformed revision number")
	}
	if revno < 0 {
		return mongodoc.ResourceRevision{}, errgo.Newf("negative revision number")
	}
	return mongodoc.ResourceRevision{
		Name:     path[0:i],
		Revision: revno,
	}, nil
}

func validResourceName(name string) bool {
	// TODO we should probably be more restrictive than this.
	return !strings.Contains(name, "/")
}
