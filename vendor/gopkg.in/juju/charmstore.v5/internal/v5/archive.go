// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v5 // import "gopkg.in/juju/charmstore.v5/internal/v5"

import (
	stdzip "archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/juju/charmstore.v5/internal/charmstore"
	"gopkg.in/juju/charmstore.v5/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5/internal/router"
)

// GET id/archive
// https://github.com/juju/charmstore/blob/v5/docs/API.md#get-idarchive
//
// POST id/archive?hash=sha384hash
// https://github.com/juju/charmstore/blob/v5/docs/API.md#post-idarchive
//
// DELETE id/archive
// https://github.com/juju/charmstore/blob/v5/docs/API.md#delete-idarchive
//
// PUT id/archive?hash=sha384hash
// This is like POST except that it puts the archive to a known revision
// rather than choosing a new one. As this feature is to support legacy
// ingestion methods, and will be removed in the future, it has no entry
// in the specification.
func (h *ReqHandler) serveArchive(id *charm.URL, w http.ResponseWriter, req *http.Request) error {
	resolveId := h.ResolvedIdHandler
	switch req.Method {
	case "DELETE":
		return resolveId(h.serveDeleteArchive)(id, w, req)
	case "GET":
		return resolveId(h.serveGetArchive)(id, w, req)
	case "POST", "PUT":
		// Make sure we consume the full request body, before responding.
		//
		// It seems a shame to require the whole, possibly large, archive
		// is uploaded if we already know that the request is going to
		// fail, but it is necessary to prevent some failures.
		//
		// TODO: investigate using 100-Continue statuses to prevent
		// unnecessary uploads.
		defer io.Copy(ioutil.Discard, req.Body)

		// Add "noingest" so that we don't need to fetch the base entity
		// twice in the common case when uploading a charm.
		h.Cache.AddBaseEntityFields(charmstore.FieldSelector("noingest"))

		if err := h.authorizeUpload(id, req); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
		if req.Method == "POST" {
			return h.servePostArchive(id, w, req)
		}
		return h.servePutArchive(id, w, req)
	}
	return errgo.WithCausef(nil, params.ErrMethodNotAllowed, "%s not allowed", req.Method)
}

func (h *ReqHandler) authorizeUpload(id *charm.URL, req *http.Request) error {
	if id.User == "" {
		return badRequestf(nil, "user not specified in entity upload URL %q", id)
	}
	baseEntity, err := h.Cache.BaseEntity(id, charmstore.FieldSelector("channelacls"))
	if err != nil && errgo.Cause(err) != params.ErrNotFound {
		return errgo.Notef(err, "cannot retrieve entity %q for authorization", id)
	}
	var acl mongodoc.ACL
	if err == nil {
		acl = baseEntity.ChannelACLs[params.UnpublishedChannel]
	} else {
		// The base entity does not currently exist, so we default to
		// assuming write permissions for the entity user.
		acl = mongodoc.ACL{
			Write: []string{id.User},
		}
	}
	// Note that we pass no entity ids to authorize, because
	// we haven't got a resolved URL at this point. At some
	// point in the future, we may want to be able to allow
	// is-entity first-party caveats to be allowed when uploading
	// at which point we will need to rethink this a little.
	if _, err := h.authorize(authorizeParams{
		req:  req,
		acls: []mongodoc.ACL{acl},
		ops:  []string{OpWrite},
	}); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	return nil
}

func (h *ReqHandler) serveGetArchive(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	if err := h.AuthorizeEntityForOp(id, req, OpReadWithTerms); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	blob, err := h.Store.OpenBlob(id)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	defer blob.Close()
	h.SendEntityArchive(id, w, req, blob)
	return nil
}

// SendEntityArchive writes the given blob, which has been retrieved
// from the given id, as a response to the given request.
func (h *ReqHandler) SendEntityArchive(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request, blob *charmstore.Blob) {
	header := w.Header()
	setArchiveCacheControl(w.Header(), h.isPublic(id))
	header.Set(params.ContentHashHeader, blob.Hash)
	header.Set(params.EntityIdHeader, id.PreferredURL().String())
	header.Set("Content-Disposition", "attachment; filename="+id.PreferredURL().Name+".zip")

	if StatsEnabled(req) {
		h.Store.IncrementDownloadCountsAsync(id)
	}
	// TODO(rog) should we set connection=close here?
	// See https://codereview.appspot.com/5958045
	serveContent(w, req, blob.Size, blob)
}

func (h *ReqHandler) serveDeleteArchive(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	if err := h.AuthorizeEntityForOp(id, req, OpWrite); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	if err := h.Store.DeleteEntity(id); err != nil {
		return errgo.NoteMask(err, fmt.Sprintf("cannot delete %q", id.PreferredURL()), errgo.Is(params.ErrNotFound), errgo.Is(params.ErrForbidden))
	}
	h.Store.IncCounterAsync(charmstore.EntityStatsKey(&id.URL, params.StatsArchiveDelete))
	return nil
}

func (h *ReqHandler) updateStatsArchiveUpload(id *charm.URL, err *error) {
	// Upload stats don't include revision: it is assumed that each
	// entity revision is only uploaded once.
	id.Revision = -1
	kind := params.StatsArchiveUpload
	if *err != nil {
		kind = params.StatsArchiveFailedUpload
	}
	h.Store.IncCounterAsync(charmstore.EntityStatsKey(id, kind))
}

func (h *ReqHandler) servePostArchive(id *charm.URL, w http.ResponseWriter, req *http.Request) (err error) {
	defer h.updateStatsArchiveUpload(id, &err)

	if id.Revision != -1 {
		return badRequestf(nil, "revision specified, but should not be specified")
	}
	if id.User == "" {
		return badRequestf(nil, "user not specified")
	}
	hash := req.Form.Get("hash")
	if hash == "" {
		return badRequestf(nil, "hash parameter not specified")
	}
	if req.ContentLength == -1 {
		return badRequestf(nil, "Content-Length not specified")
	}
	oldURL, oldHash, err := h.latestRevisionInfo(id)
	if err != nil && errgo.Cause(err) != params.ErrNotFound {
		return errgo.Notef(err, "cannot get hash of latest revision")
	}
	if oldHash == hash {
		// The hash matches the hash of the latest revision, so
		// no need to upload anything.
		return httprequest.WriteJSON(w, http.StatusOK, &params.ArchiveUploadResponse{
			Id:            &oldURL.URL,
			PromulgatedId: oldURL.PromulgatedURL(),
		})
	}
	newRevision, err := h.Store.NewRevision(id)
	if err != nil {
		return errgo.Notef(err, "cannot get new revision")
	}
	rid := &router.ResolvedURL{URL: *id}
	rid.URL.Revision = newRevision
	rid.PromulgatedRevision, err = h.getNewPromulgatedRevision(id)
	if err != nil {
		return errgo.Mask(err)
	}
	if err := h.Store.UploadEntity(rid, req.Body, hash, req.ContentLength, nil); err != nil {
		return errgo.Mask(err,
			errgo.Is(params.ErrDuplicateUpload),
			errgo.Is(params.ErrEntityIdNotAllowed),
			errgo.Is(params.ErrInvalidEntity),
		)
	}
	if ingesting, _ := router.ParseBool(req.Form.Get("ingest")); !ingesting {
		// Find the base entity before trying to update its noingest status
		// as if this isn't the first upload of the entity, the base entity
		// will already be cached, so we won't need any more round trips
		// to mongo than usual.
		baseEntity, err := h.Cache.BaseEntity(&rid.URL, charmstore.FieldSelector("noingest"))
		if err != nil || !baseEntity.NoIngest {
			if err := h.Store.DB.BaseEntities().UpdateId(mongodoc.BaseURL(&rid.URL), bson.D{{
				"$set", bson.D{{
					"noingest", true,
				}},
			}}); err != nil {
				// We can't update NoIngest status but that's probably no big deal
				// so just log the error and move on.
				logger.Errorf("cannot update NoIngest status of entity %v: %v", &rid.URL, err)
			}
		}
	}
	return httprequest.WriteJSON(w, http.StatusOK, &params.ArchiveUploadResponse{
		Id:            &rid.URL,
		PromulgatedId: rid.PromulgatedURL(),
	})
}

func (h *ReqHandler) servePutArchive(id *charm.URL, w http.ResponseWriter, req *http.Request) (err error) {
	defer h.updateStatsArchiveUpload(id, &err)
	if id.Series == "" {
		return badRequestf(nil, "series not specified")
	}
	if id.Revision == -1 {
		return badRequestf(nil, "revision not specified")
	}
	if id.User == "" {
		return badRequestf(nil, "user not specified")
	}
	hash := req.Form.Get("hash")
	if hash == "" {
		return badRequestf(nil, "hash parameter not specified")
	}
	if req.ContentLength == -1 {
		return badRequestf(nil, "Content-Length not specified")
	}
	var chans []params.Channel
	for _, c := range req.Form["channel"] {
		c := params.Channel(c)
		if !params.ValidChannels[c] || c == params.UnpublishedChannel {
			return badRequestf(nil, "cannot put entity into channel %q", c)
		}
		chans = append(chans, c)
	}
	rid := &router.ResolvedURL{
		URL:                 *id,
		PromulgatedRevision: -1,
	}
	// Get the PromulgatedURL from the request parameters. When ingesting
	// entities might not be added in order and the promulgated revision might
	// not match the non-promulgated revision, so the full promulgated URL
	// needs to be specified.
	promulgatedURL := req.Form.Get("promulgated")
	var pid *charm.URL
	if promulgatedURL != "" {
		pid, err = charm.ParseURL(promulgatedURL)
		if err != nil {
			return badRequestf(err, "cannot parse promulgated url")
		}
		if pid.User != "" {
			return badRequestf(nil, "promulgated URL cannot have a user")
		}
		if pid.Name != id.Name {
			return badRequestf(nil, "promulgated URL has incorrect charm name")
		}
		if pid.Series != id.Series {
			return badRequestf(nil, "promulgated URL has incorrect series")
		}
		if pid.Revision == -1 {
			return badRequestf(nil, "promulgated URL has no revision")
		}
		rid.PromulgatedRevision = pid.Revision
	}
	// Register the new revisions we're about to use.
	if err := h.Store.AddRevision(rid); err != nil {
		return errgo.Mask(err)
	}
	if err := h.Store.UploadEntity(rid, req.Body, hash, req.ContentLength, chans); err != nil {
		return errgo.Mask(err,
			errgo.Is(params.ErrDuplicateUpload),
			errgo.Is(params.ErrEntityIdNotAllowed),
			errgo.Is(params.ErrInvalidEntity),
		)
	}
	return httprequest.WriteJSON(w, http.StatusOK, &params.ArchiveUploadResponse{
		Id:            &rid.URL,
		PromulgatedId: rid.PromulgatedURL(),
	})
	return nil
}

func (h *ReqHandler) latestRevisionInfo(id *charm.URL) (*router.ResolvedURL, string, error) {
	entities, err := h.Store.FindEntities(id, charmstore.FieldSelector("_id", "blobhash", "promulgated-url"))
	if err != nil {
		return nil, "", errgo.Mask(err)
	}
	if len(entities) == 0 {
		return nil, "", params.ErrNotFound
	}
	latest := entities[0]
	for _, entity := range entities {
		if entity.URL.Revision > latest.URL.Revision {
			latest = entity
		}
	}
	return charmstore.EntityResolvedURL(latest), latest.BlobHash, nil
}

// GET id/archive/path
// https://github.com/juju/charmstore/blob/v5/docs/API.md#get-idarchivepath
func (h *ReqHandler) serveArchiveFile(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	blob, err := h.Store.OpenBlob(id)
	if err != nil {
		return errgo.Notef(err, "cannot open archive data for %v", id)
	}
	defer blob.Close()
	return h.ServeBlobFile(w, req, id, blob)
}

// ServeBlobFile serves a file from the given blob. The
// path of the file is taken from req.URL.Path.
// The blob should be associated with the entity
// with the given id.
func (h *ReqHandler) ServeBlobFile(w http.ResponseWriter, req *http.Request, id *router.ResolvedURL, blob *charmstore.Blob) error {
	r, size, err := h.Store.OpenBlobFile(blob, req.URL.Path)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrForbidden))
	}
	defer r.Close()
	ctype := mime.TypeByExtension(filepath.Ext(req.URL.Path))
	if ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	setArchiveCacheControl(w.Header(), h.isPublic(id))
	w.WriteHeader(http.StatusOK)
	io.Copy(w, r)
	return nil
}

func (h *ReqHandler) isPublic(id *router.ResolvedURL) bool {
	acls, _ := h.entityACLs(id)
	for _, p := range acls.Read {
		if p == params.Everyone {
			return true
		}
	}
	return false
}

// ArchiveCachePublicMaxAge specifies the cache expiry duration for items
// returned from the archive where the id represents the id of a public entity.
const ArchiveCachePublicMaxAge = 1 * time.Hour

// setArchiveCacheControl sets cache control headers
// in a response to an archive-derived endpoint.
// The isPublic parameter specifies whether
// the entity id can or not be cached .
func setArchiveCacheControl(h http.Header, isPublic bool) {
	if isPublic {
		seconds := int(ArchiveCachePublicMaxAge / time.Second)
		h.Set("Cache-Control", "public, max-age="+strconv.Itoa(seconds))
	} else {
		h.Set("Cache-Control", "no-cache, must-revalidate")
	}
}

// getNewPromulgatedRevision returns the promulgated revision
// to give to a newly uploaded charm with the given id.
// It returns -1 if the charm is not promulgated.
func (h *ReqHandler) getNewPromulgatedRevision(id *charm.URL) (int, error) {
	baseEntity, err := h.Cache.BaseEntity(id, charmstore.FieldSelector("promulgated"))
	if err != nil && errgo.Cause(err) != params.ErrNotFound {
		return 0, errgo.Mask(err)
	}
	if baseEntity == nil || !baseEntity.Promulgated {
		return -1, nil
	}
	purl := &charm.URL{
		Schema:   "cs",
		Series:   id.Series,
		Name:     id.Name,
		Revision: -1,
	}
	rev, err := h.Store.NewRevision(purl)
	if err != nil {
		return 0, errgo.Mask(err)
	}
	return rev, nil
}

// archiveReadError creates an appropriate error for errors in reading an
// uploaded archive. If the archive could not be read because the data
// uploaded is invalid then an error with a cause of
// params.ErrInvalidEntity will be returned. The given message will be
// added as context.
func archiveReadError(err error, msg string) error {
	switch errgo.Cause(err) {
	case stdzip.ErrFormat, stdzip.ErrAlgorithm, stdzip.ErrChecksum:
		return errgo.WithCausef(err, params.ErrInvalidEntity, msg)
	}
	return errgo.Notef(err, msg)
}
