// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v4 // import "gopkg.in/juju/charmstore.v5/internal/v4"

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/juju/loggo"
	"github.com/juju/mempool"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/juju/charmstore.v5/internal/charmstore"
	"gopkg.in/juju/charmstore.v5/internal/entitycache"
	"gopkg.in/juju/charmstore.v5/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5/internal/router"
	"gopkg.in/juju/charmstore.v5/internal/v5"
)

var logger = loggo.GetLogger("charmstore.internal.v4")

// reqHandlerPool holds a cache of ReqHandlers to save
// on allocation time. When a handler is done with,
// it is put back into the pool.
var reqHandlerPool = mempool.Pool{
	New: func() interface{} {
		return newReqHandler()
	},
}

type Handler struct {
	*v5.Handler
}

type ReqHandler struct {
	*v5.ReqHandler
}

func New(p charmstore.APIHandlerParams) (Handler, error) {
	h, err := v5.New(p)
	if err != nil {
		return Handler{}, errgo.Mask(err)
	}
	return Handler{
		Handler: h,
	}, nil
}

func (h Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rh, err := h.NewReqHandler(req)
	if err != nil {
		router.WriteError(context.TODO(), w, err)
		return
	}
	defer rh.Close()
	rh.Router.Monitor.Reset(req.Method, "v4")
	defer rh.Router.Monitor.Done()
	rh.ServeHTTP(w, req)
}

func NewAPIHandler(p charmstore.APIHandlerParams) (charmstore.HTTPCloseHandler, error) {
	h, err := New(p)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return h, nil
}

// The v4 resolvedURL function also requires SupportedSeries.
var requiredEntityFields = func() map[string]int {
	fields := make(map[string]int)
	for f := range v5.RequiredEntityFields {
		fields[f] = 1
	}
	fields["supportedseries"] = 1
	return fields
}()

// NewReqHandler returns an instance of a *ReqHandler
// suitable for handling the given HTTP request. After use, the ReqHandler.Close
// method should be called to close it.
//
// If no handlers are available, it returns an error with
// a charmstore.ErrTooManySessions cause.
func (h *Handler) NewReqHandler(req *http.Request) (ReqHandler, error) {
	req.ParseForm()
	// Validate all the values for channel, even though
	// most endpoints will only ever use the first one.
	// PUT to an archive is the notable exception.
	// TODO Why is the v4 API accepting a channel parameter anyway? We
	// should probably always use "stable".
	for _, ch := range req.Form["channel"] {
		if !params.ValidChannels[params.Channel(ch)] {
			return ReqHandler{}, badRequestf(nil, "invalid channel %q specified in request", ch)
		}
	}
	store, err := h.Pool.RequestStore()
	if err != nil {
		if errgo.Cause(err) == charmstore.ErrTooManySessions {
			return ReqHandler{}, errgo.WithCausef(err, params.ErrServiceUnavailable, "")
		}
		return ReqHandler{}, errgo.Mask(err)
	}
	rh := reqHandlerPool.Get().(ReqHandler)
	rh.Handler = h.Handler
	rh.Store = &v5.StoreWithChannel{
		Store:   store,
		Channel: params.Channel(req.Form.Get("channel")),
	}
	rh.Cache = entitycache.New(rh.Store)
	rh.Cache.AddEntityFields(requiredEntityFields)
	rh.Cache.AddBaseEntityFields(v5.RequiredBaseEntityFields)
	return rh, nil
}

func newReqHandler() ReqHandler {
	h := ReqHandler{
		ReqHandler: new(v5.ReqHandler),
	}
	resolveId := h.ResolvedIdHandler
	authId := h.AuthIdHandler
	handlers := v5.RouterHandlers(h.ReqHandler)
	handlers.Global["search"] = router.HandleJSON(h.serveSearch)
	handlers.Meta["bundle-metadata"] = h.EntityHandler(h.metaBundleMetadata, "bundledata")
	handlers.Meta["charm-related"] = h.EntityHandler(h.metaCharmRelated, "charmprovidedinterfaces", "charmrequiredinterfaces")
	handlers.Meta["charm-metadata"] = h.EntityHandler(h.metaCharmMetadata, "charmmeta")
	handlers.Meta["revision-info"] = router.SingleIncludeHandler(h.metaRevisionInfo)
	handlers.Meta["archive-size"] = h.EntityHandler(h.metaArchiveSize, "prev5blobsize")
	handlers.Meta["hash"] = h.EntityHandler(h.metaHash, "prev5blobhash")
	handlers.Meta["hash256"] = h.EntityHandler(h.metaHash256, "prev5blobhash256")
	handlers.Id["expand-id"] = resolveId(authId(h.serveExpandId))
	handlers.Id["archive"] = h.serveArchive(handlers.Id["archive"])
	handlers.Id["archive/"] = resolveId(authId(h.serveArchiveFile))

	// Delete new endpoints that we don't want to provide in v4.
	delete(handlers.Id, "publish")
	delete(handlers.Meta, "published")
	delete(handlers.Id, "resource")
	delete(handlers.Meta, "resources")
	delete(handlers.Meta, "resources/")
	delete(handlers.Meta, "can-ingest")
	delete(handlers.Meta, "can-write")
	delete(handlers.Global, "upload")
	delete(handlers.Global, "upload/")

	h.Router = router.New(handlers, h)
	return h
}

// ResolveURL implements router.Context.ResolveURL,
// ensuring that any resulting ResolvedURL always
// has a non-empty PreferredSeries field.
func (h ReqHandler) ResolveURL(url *charm.URL) (*router.ResolvedURL, error) {
	return resolveURL(h.Cache, url)
}

func (h ReqHandler) ResolveURLs(urls []*charm.URL) ([]*router.ResolvedURL, error) {
	h.Cache.StartFetch(urls)
	rurls := make([]*router.ResolvedURL, len(urls))
	for i, url := range urls {
		var err error
		rurls[i], err = resolveURL(h.Cache, url)
		if err != nil && errgo.Cause(err) != params.ErrNotFound {
			return nil, err
		}
	}
	return rurls, nil
}

// resolveURL implements URL resolving for the ReqHandler.
// It's defined as a separate function so it can be more
// easily unit-tested.
func resolveURL(cache *entitycache.Cache, url *charm.URL) (*router.ResolvedURL, error) {
	entity, err := cache.Entity(url, charmstore.FieldSelector("supportedseries"))
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	rurl := &router.ResolvedURL{
		URL:                 *entity.URL,
		PromulgatedRevision: -1,
	}
	if url.User == "" {
		rurl.PromulgatedRevision = entity.PromulgatedRevision
	}
	if rurl.URL.Series != "" {
		return rurl, nil
	}
	if url.Series != "" {
		rurl.PreferredSeries = url.Series
		return rurl, nil
	}
	if len(entity.SupportedSeries) == 0 {
		return nil, errgo.Newf("entity %q has no supported series", &rurl.URL)
	}
	rurl.PreferredSeries = entity.SupportedSeries[0]
	return rurl, nil
}

// Close closes the ReqHandler. This should always be called when the
// ReqHandler is done with.
func (h ReqHandler) Close() {
	h.Store.Close()
	h.Cache.Close()
	h.Reset()
	reqHandlerPool.Put(h)
}

// GET id/meta/bundle-metadata
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmetabundle-metadata
func (h ReqHandler) metaBundleMetadata(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	m := entity.BundleData
	if m == nil {
		return nil, nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, errgo.Notef(err, "cannot marshal bundle-metadata")
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal bundle-metadata")
	}
	if ap, ok := metadata["applications"]; ok {
		metadata["Services"] = ap
		delete(metadata, "applications")
	}
	return metadata, nil
}

// GET id/meta/charm-metadata
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmetacharm-metadata
func (h ReqHandler) metaCharmMetadata(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	m := entity.CharmMeta
	if m != nil {
		m.Series = nil
	}
	return m, nil
}

// GET id/meta/revision-info
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmetarevision-info
func (h ReqHandler) metaRevisionInfo(id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	searchURL := id.PreferredURL()
	searchURL.Revision = -1

	q := h.Store.EntitiesQuery(searchURL)
	if id.PromulgatedRevision != -1 {
		q = q.Sort("-promulgated-revision")
	} else {
		q = q.Sort("-revision")
	}
	var docs []*mongodoc.Entity
	if err := q.Select(bson.D{{"_id", 1}, {"promulgated-url", 1}, {"supportedseries", 1}}).All(&docs); err != nil {
		return "", errgo.Notef(err, "cannot get ids")
	}

	if len(docs) == 0 {
		return "", errgo.WithCausef(nil, params.ErrNotFound, "no matching charm or bundle for %s", id)
	}
	specifiedSeries := id.URL.Series
	if specifiedSeries == "" {
		specifiedSeries = id.PreferredSeries
	}
	var response params.RevisionInfoResponse
	expandMultiSeries(docs, func(series string, doc *mongodoc.Entity) error {
		if specifiedSeries != series {
			return nil
		}
		url := doc.PreferredURL(id.PromulgatedRevision != -1)
		url.Series = series
		response.Revisions = append(response.Revisions, url)
		return nil
	})
	return &response, nil
}

// GET id/meta/archive-size
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmetaarchive-size
func (h ReqHandler) metaArchiveSize(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return &params.ArchiveSizeResponse{
		Size: entity.PreV5BlobSize,
	}, nil
}

// GET id/meta/hash
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmetahash
func (h ReqHandler) metaHash(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return &params.HashResponse{
		Sum: entity.PreV5BlobHash,
	}, nil
}

// GET id/meta/hash256
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmetahash256
func (h ReqHandler) metaHash256(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return &params.HashResponse{
		Sum: entity.PreV5BlobHash256,
	}, nil
}

// GET id/expand-id
// https://docs.google.com/a/canonical.com/document/d/1TgRA7jW_mmXoKH3JiwBbtPvQu7WiM6XMrz1wSrhTMXw/edit#bookmark=id.4xdnvxphb2si
func (h ReqHandler) serveExpandId(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	baseURL := id.PreferredURL()
	baseURL.Revision = -1
	baseURL.Series = ""

	// baseURL now represents the base URL of the given id;
	// it will be a promulgated URL iff the original URL was
	// specified without a user, which will cause EntitiesQuery
	// to return entities that match appropriately.

	// Retrieve all the entities with the same base URL.
	q := h.Store.EntitiesQuery(baseURL).Select(bson.D{{"_id", 1}, {"promulgated-url", 1}, {"supportedseries", 1}})
	if id.PromulgatedRevision != -1 {
		q = q.Sort("-series", "-promulgated-revision")
	} else {
		q = q.Sort("-series", "-revision")
	}
	var docs []*mongodoc.Entity
	err := q.All(&docs)
	if err != nil && errgo.Cause(err) != mgo.ErrNotFound {
		return errgo.Mask(err)
	}

	// Collect all the expanded identifiers for each entity.
	response := make([]params.ExpandedId, 0, len(docs))
	expandMultiSeries(docs, func(series string, doc *mongodoc.Entity) error {
		if err := h.AuthorizeEntity(charmstore.EntityResolvedURL(doc), req); err != nil {
			return nil
		}
		url := doc.PreferredURL(id.PromulgatedRevision != -1)
		url.Series = series
		response = append(response, params.ExpandedId{Id: url.String()})
		return nil
	})

	// Write the response in JSON format.
	return httprequest.WriteJSON(w, http.StatusOK, response)
}

// expandMultiSeries calls the provided append function once for every
// supported series of each entry in the given entities slice. The series
// argument will be passed as that series and the doc argument will point
// to the entity. This function will only return an error if the append
// function returns an error; such an error will be returned without
// masking the cause.
//
// Note that the SupportedSeries field of the entities must have
// been populated for this to work.
func expandMultiSeries(entities []*mongodoc.Entity, append func(series string, doc *mongodoc.Entity) error) error {
	// TODO(rog) make this concurrent.
	for _, entity := range entities {
		if entity.URL.Series != "" {
			append(entity.URL.Series, entity)
			continue
		}
		for _, series := range entity.SupportedSeries {
			if err := append(series, entity); err != nil {
				return errgo.Mask(err, errgo.Any)
			}
		}
	}
	return nil
}

func badRequestf(underlying error, f string, a ...interface{}) error {
	err := errgo.WithCausef(underlying, params.ErrBadRequest, f, a...)
	err.(*errgo.Err).SetLocation(1)
	return err
}
