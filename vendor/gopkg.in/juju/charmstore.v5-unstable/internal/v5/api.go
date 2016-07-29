// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v5 // import "gopkg.in/juju/charmstore.v5-unstable/internal/v5"

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/juju/httprequest"
	"github.com/juju/idmclient"
	"github.com/juju/loggo"
	"github.com/juju/mempool"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/juju/charmstore.v5-unstable/audit"
	"gopkg.in/juju/charmstore.v5-unstable/internal/agent"
	"gopkg.in/juju/charmstore.v5-unstable/internal/cache"
	"gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"
	"gopkg.in/juju/charmstore.v5-unstable/internal/entitycache"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
)

var logger = loggo.GetLogger("charmstore.internal.v5")

// reqHandlerPool holds a cache of ReqHandlers to save
// on allocation time. When a handler is done with,
// it is put back into the pool.
var reqHandlerPool = mempool.Pool{
	New: func() interface{} {
		return newReqHandler()
	},
}

type Handler struct {
	// Pool holds the store pool that the handler was created
	// with.
	Pool *charmstore.Pool

	config      charmstore.ServerParams
	locator     bakery.PublicKeyLocator
	groupCache  groupCache
	permChecker permChecker
	rootPath    string

	// searchCache is a cache of search results keyed on the query
	// parameters of the search. It should only be used for searches
	// from unauthenticated users.
	searchCache *cache.Cache
}

// ReqHandler holds the context for a single HTTP request.
// It uses an independent mgo session from the handler
// used by other requests.
type ReqHandler struct {
	// Router holds the router that the ReqHandler will use
	// to route HTTP requests. This is usually set by
	// Handler.NewReqHandler to the result of RouterHandlers.
	Router *router.Router

	// Handler holds the Handler that the ReqHandler
	// is derived from.
	Handler *Handler

	// Store holds the charmstore Store instance
	// for the request, associated with the channel specified
	// in the request.
	Store *StoreWithChannel

	// auth holds the results of any authorization that
	// has been done on this request.
	auth authorization

	// cache holds the per-request entity cache.
	Cache *entitycache.Cache
}

const (
	DelegatableMacaroonExpiry = time.Minute
	reqHandlerCacheSize       = 50
)

// PermCacheExpiry holds the maximum length of time that permissions
// and group membership information will be cached for.
var PermCacheExpiry = time.Minute

type permChecker interface {
	Allow(username string, acl []string) (bool, error)
}

type groupCache interface {
	Groups(username string) ([]string, error)
}

func New(pool *charmstore.Pool, config charmstore.ServerParams, rootPath string) *Handler {
	var groupCache groupCache
	var permChecker permChecker
	if config.IdentityAPIURL != "" && config.AgentUsername != "" && config.AgentKey != nil {
		logger.Infof("group membership enabled at %s; agent %s", config.IdentityAPIURL, config.AgentUsername)
		identityClient := idmclient.New(idmclient.NewParams{
			BaseURL: config.IdentityAPIURL,
			Client:  agent.NewClient(config.AgentUsername, config.AgentKey),
		})
		gc := idmclient.NewGroupCache(identityClient, PermCacheExpiry)
		groupCache = gc
		permChecker = idmclient.NewPermCheckerWithCache(gc)
	} else {
		logger.Infof("group membership not enabled (no identity-api-url or agent-username)")
		groupCache = noGroupCache{}
		permChecker = noGroupsPermChecker{}
	}
	h := &Handler{
		Pool:        pool,
		config:      config,
		rootPath:    rootPath,
		searchCache: cache.New(config.SearchCacheMaxAge),
		locator:     config.PublicKeyLocator,
		groupCache:  groupCache,
		permChecker: permChecker,
	}
	return h
}

// Close closes the Handler.
func (h *Handler) Close() {
}

var (
	RequiredEntityFields = charmstore.FieldSelector(
		"baseurl",
		"user",
		"name",
		"revision",
		"series",
		"promulgated-revision",
		"promulgated-url",
		"development",
		"stable",
	)
	RequiredBaseEntityFields = charmstore.FieldSelector(
		"user",
		"name",
		"channelacls",
		"channelentities",
		"promulgated",
	)
)

// StoreWithChannel associates a Store with a channel that will be used
// to resolve any channel-ambiguous requests.
type StoreWithChannel struct {
	*charmstore.Store
	Channel params.Channel
}

func (s *StoreWithChannel) FindBestEntity(url *charm.URL, fields map[string]int) (*mongodoc.Entity, error) {
	return s.Store.FindBestEntity(url, s.Channel, fields)
}

func (s *StoreWithChannel) FindBaseEntity(url *charm.URL, fields map[string]int) (*mongodoc.BaseEntity, error) {
	return s.Store.FindBaseEntity(url, fields)
}

// ValidChannels holds the set of all allowed channels
// that can be passed as a "?channel=" parameter.
var ValidChannels = map[params.Channel]bool{
	params.UnpublishedChannel: true,
	params.DevelopmentChannel: true,
	params.StableChannel:      true,
}

// NewReqHandler returns an instance of a *ReqHandler
// suitable for handling the given HTTP request. After use, the ReqHandler.Close
// method should be called to close it.
//
// If no handlers are available, it returns an error with
// a charmstore.ErrTooManySessions cause.
func (h *Handler) NewReqHandler(req *http.Request) (*ReqHandler, error) {
	req.ParseForm()
	// Validate all the values for channel, even though
	// most endpoints will only ever use the first one.
	// PUT to an archive is the notable exception.
	for _, ch := range req.Form["channel"] {
		if !ValidChannels[params.Channel(ch)] {
			return nil, badRequestf(nil, "invalid channel %q specified in request", ch)
		}
	}
	store, err := h.Pool.RequestStore()
	if err != nil {
		if errgo.Cause(err) == charmstore.ErrTooManySessions {
			return nil, errgo.WithCausef(err, params.ErrServiceUnavailable, "")
		}
		return nil, errgo.Mask(err)
	}
	rh := reqHandlerPool.Get().(*ReqHandler)
	rh.Handler = h
	rh.Store = &StoreWithChannel{
		Store:   store,
		Channel: params.Channel(req.Form.Get("channel")),
	}
	rh.Cache = entitycache.New(rh.Store)
	rh.Cache.AddEntityFields(RequiredEntityFields)
	rh.Cache.AddBaseEntityFields(RequiredBaseEntityFields)
	return rh, nil
}

// RouterHandlers returns router handlers that will route requests to
// the given ReqHandler. This is provided so that different API versions
// can override selected parts of the handlers to serve their own API
// while still using ReqHandler to serve the majority of the API.
func RouterHandlers(h *ReqHandler) *router.Handlers {
	resolveId := h.ResolvedIdHandler
	authId := h.AuthIdHandler
	return &router.Handlers{
		Global: map[string]http.Handler{
			"changes/published":    router.HandleJSON(h.serveChangesPublished),
			"debug":                http.HandlerFunc(h.serveDebug),
			"debug/pprof/":         newPprofHandler(h),
			"debug/status":         router.HandleJSON(h.serveDebugStatus),
			"list":                 router.HandleJSON(h.serveList),
			"log":                  router.HandleErrors(h.serveLog),
			"logout":               http.HandlerFunc(logout),
			"search":               router.HandleJSON(h.serveSearch),
			"search/interesting":   http.HandlerFunc(h.serveSearchInteresting),
			"set-auth-cookie":      router.HandleErrors(h.serveSetAuthCookie),
			"stats/":               router.NotFoundHandler(),
			"stats/counter/":       router.HandleJSON(h.serveStatsCounter),
			"stats/update":         router.HandleErrors(h.serveStatsUpdate),
			"macaroon":             router.HandleJSON(h.serveMacaroon),
			"delegatable-macaroon": router.HandleJSON(h.serveDelegatableMacaroon),
			"whoami":               router.HandleJSON(h.serveWhoAmI),
		},
		Id: map[string]router.IdHandler{
			"archive":     h.serveArchive,
			"archive/":    resolveId(authId(h.serveArchiveFile), "blobname", "blobhash"),
			"diagram.svg": resolveId(authId(h.serveDiagram), "bundledata"),
			"expand-id":   resolveId(authId(h.serveExpandId)),
			"icon.svg":    resolveId(authId(h.serveIcon), "contents", "blobname"),
			"publish":     resolveId(h.servePublish),
			"promulgate":  resolveId(h.servePromulgate),
			"readme":      resolveId(authId(h.serveReadMe), "contents", "blobname"),
			"resource/":   resolveId(authId(h.serveResources), "charmmeta"),
		},
		Meta: map[string]router.BulkIncludeHandler{
			"archive-size":         h.EntityHandler(h.metaArchiveSize, "size"),
			"archive-upload-time":  h.EntityHandler(h.metaArchiveUploadTime, "uploadtime"),
			"bundle-machine-count": h.EntityHandler(h.metaBundleMachineCount, "bundlemachinecount"),
			"bundle-metadata":      h.EntityHandler(h.metaBundleMetadata, "bundledata"),
			"bundles-containing":   h.EntityHandler(h.metaBundlesContaining),
			"bundle-unit-count":    h.EntityHandler(h.metaBundleUnitCount, "bundleunitcount"),
			"published":            h.EntityHandler(h.metaPublished, "development", "stable"),
			"charm-actions":        h.EntityHandler(h.metaCharmActions, "charmactions"),
			"charm-config":         h.EntityHandler(h.metaCharmConfig, "charmconfig"),
			"charm-metadata":       h.EntityHandler(h.metaCharmMetadata, "charmmeta"),
			"charm-related":        h.EntityHandler(h.metaCharmRelated, "charmprovidedinterfaces", "charmrequiredinterfaces"),
			"common-info": h.puttableBaseEntityHandler(
				h.metaCommonInfo,
				h.putMetaCommonInfo,
				"commoninfo",
			),
			"common-info/": h.puttableBaseEntityHandler(
				h.metaCommonInfoWithKey,
				h.putMetaCommonInfoWithKey,
				"commoninfo",
			),
			"extra-info": h.puttableEntityHandler(
				h.metaExtraInfo,
				h.putMetaExtraInfo,
				"extrainfo",
			),
			"extra-info/": h.puttableEntityHandler(
				h.metaExtraInfoWithKey,
				h.putMetaExtraInfoWithKey,
				"extrainfo",
			),
			"hash":             h.EntityHandler(h.metaHash, "blobhash"),
			"hash256":          h.EntityHandler(h.metaHash256, "blobhash256"),
			"id":               h.EntityHandler(h.metaId, "_id"),
			"id-name":          h.EntityHandler(h.metaIdName, "_id"),
			"id-user":          h.EntityHandler(h.metaIdUser, "_id"),
			"id-revision":      h.EntityHandler(h.metaIdRevision, "_id"),
			"id-series":        h.EntityHandler(h.metaIdSeries, "_id"),
			"manifest":         h.EntityHandler(h.metaManifest, "blobname"),
			"owner":            h.EntityHandler(h.metaOwner, "_id"),
			"perm":             h.puttableBaseEntityHandler(h.metaPerm, h.putMetaPerm, "channelacls"),
			"perm/":            h.puttableBaseEntityHandler(h.metaPermWithKey, h.putMetaPermWithKey, "channelacls"),
			"promulgated":      h.baseEntityHandler(h.metaPromulgated, "promulgated"),
			"can-ingest":       h.baseEntityHandler(h.metaCanIngest, "noingest"),
			"resources":        h.EntityHandler(h.metaResources, "charmmeta"),
			"resources/":       h.EntityHandler(h.metaResourcesSingle, "charmmeta"),
			"revision-info":    router.SingleIncludeHandler(h.metaRevisionInfo),
			"stats":            h.EntityHandler(h.metaStats),
			"supported-series": h.EntityHandler(h.metaSupportedSeries, "supportedseries"),
			"tags":             h.EntityHandler(h.metaTags, "charmmeta", "bundledata"),
			"terms":            h.EntityHandler(h.metaTerms, "charmmeta"),

			// endpoints not yet implemented:
			// "color": router.SingleIncludeHandler(h.metaColor),
		},
	}
}

// newReqHandler returns a new instance of the v4 API handler.
// The returned value has nil handler and store fields.
func newReqHandler() *ReqHandler {
	var h ReqHandler
	h.Router = router.New(RouterHandlers(&h), &h)
	return &h
}

// ServeHTTP implements http.Handler by first retrieving a
// request-specific instance of ReqHandler and
// calling ServeHTTP on that.
func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rh, err := h.NewReqHandler(req)
	if err != nil {
		router.WriteError(w, err)
		return
	}
	defer rh.Close()
	rh.ServeHTTP(w, req)
}

// GroupsForUser returns all the groups that the given user
// is a member of.
func (h *Handler) GroupsForUser(username string) ([]string, error) {
	return h.groupCache.Groups(username)
}

// ServeHTTP implements http.Handler by calling h.Router.ServeHTTP.
func (h *ReqHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h.Router.ServeHTTP(w, req)
}

// NewAPIHandler returns a new Handler as an http Handler.
// It is defined for the convenience of callers that require a
// charmstore.NewAPIHandlerFunc.
func NewAPIHandler(pool *charmstore.Pool, config charmstore.ServerParams, rootPath string) charmstore.HTTPCloseHandler {
	return New(pool, config, rootPath)
}

// Close closes the ReqHandler. This should always be called when the
// ReqHandler is done with.
func (h *ReqHandler) Close() {
	h.Store.Close()
	h.Cache.Close()
	h.Reset()
	reqHandlerPool.Put(h)
}

// Reset resets the request-specific fields of the ReqHandler
// so that it's suitable for putting back into a pool for reuse.
func (h *ReqHandler) Reset() {
	h.Store = nil
	h.Handler = nil
	h.Cache = nil
	h.auth = authorization{}
}

// ResolveURL implements router.Context.ResolveURL.
func (h *ReqHandler) ResolveURL(url *charm.URL) (*router.ResolvedURL, error) {
	return resolveURL(h.Cache, url)
}

// ResolveURL implements router.Context.ResolveURLs.
func (h *ReqHandler) ResolveURLs(urls []*charm.URL) ([]*router.ResolvedURL, error) {
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

// WillIncludeMetadata implements router.Context.WillIncludeMetadata.
func (h *ReqHandler) WillIncludeMetadata(includes []string) {
	for _, inc := range includes {
		// Find what handler will be used for the include
		// and prime the cache so that it will preemptively fetch
		// any fields involved.
		fi, ok := h.Router.MetaHandler(inc).(*router.FieldIncludeHandler)
		if !ok || len(fi.P.Fields) == 0 {
			continue
		}
		fields := make(map[string]int)
		for _, f := range fi.P.Fields {
			fields[f] = 1
		}
		switch fi.P.Key.(type) {
		case entityHandlerKey:
			h.Cache.AddEntityFields(fields)
		case baseEntityHandlerKey:
			h.Cache.AddBaseEntityFields(fields)
		}
	}
}

// resolveURL implements URL resolving for the ReqHandler.
// It's defined as a separate function so it can be more
// easily unit-tested.
func resolveURL(cache *entitycache.Cache, url *charm.URL) (*router.ResolvedURL, error) {
	// We've added promulgated-url as a required field, so
	// we'll always get it from the Entity result.
	entity, err := cache.Entity(url, nil)
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
	// Ensure the base URL is in the cache too, so that
	// its canonical URL is in the cache, so that when
	// we come to look up the base URL from the resolved
	// URL, it will hit the cached base entity.
	// We don't actually care if it succeeds or fails, so we ignore
	// the result.
	cache.BaseEntity(entity.BaseURL, nil)
	return rurl, nil
}

type EntityHandlerFunc func(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error)

type baseEntityHandlerFunc func(entity *mongodoc.BaseEntity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error)

// EntityHandler returns a Handler that calls f with a *mongodoc.Entity that
// contains at least the given fields. It allows only GET requests.
func (h *ReqHandler) EntityHandler(f EntityHandlerFunc, fields ...string) router.BulkIncludeHandler {
	return h.puttableEntityHandler(f, nil, fields...)
}

type entityHandlerKey struct{}

func (h *ReqHandler) puttableEntityHandler(get EntityHandlerFunc, handlePut router.FieldPutFunc, fields ...string) router.BulkIncludeHandler {
	handleGet := func(doc interface{}, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
		edoc := doc.(*mongodoc.Entity)
		val, err := get(edoc, id, path, flags, req)
		return val, errgo.Mask(err, errgo.Any)
	}
	return router.NewFieldIncludeHandler(router.FieldIncludeHandlerParams{
		Key:          entityHandlerKey{},
		Query:        h.entityQuery,
		Fields:       fields,
		HandleGet:    handleGet,
		HandlePut:    handlePut,
		Update:       h.updateEntity,
		UpdateSearch: h.updateSearch,
	})
}

// baseEntityHandler returns a Handler that calls f with a *mongodoc.Entity that
// contains at least the given fields. It allows only GET requests.
func (h *ReqHandler) baseEntityHandler(f baseEntityHandlerFunc, fields ...string) router.BulkIncludeHandler {
	return h.puttableBaseEntityHandler(f, nil, fields...)
}

type baseEntityHandlerKey struct{}

func (h *ReqHandler) puttableBaseEntityHandler(get baseEntityHandlerFunc, handlePut router.FieldPutFunc, fields ...string) router.BulkIncludeHandler {
	handleGet := func(doc interface{}, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
		edoc := doc.(*mongodoc.BaseEntity)
		val, err := get(edoc, id, path, flags, req)
		return val, errgo.Mask(err, errgo.Any)
	}
	return router.NewFieldIncludeHandler(router.FieldIncludeHandlerParams{
		Key:          baseEntityHandlerKey{},
		Query:        h.baseEntityQuery,
		Fields:       fields,
		HandleGet:    handleGet,
		HandlePut:    handlePut,
		Update:       h.updateBaseEntity,
		UpdateSearch: h.updateSearchBase,
	})
}

func (h *ReqHandler) processEntries(entries []audit.Entry) {
	for _, e := range entries {
		h.addAudit(e)
	}
}

func (h *ReqHandler) updateBaseEntity(id *router.ResolvedURL, fields map[string]interface{}, entries []audit.Entry) error {
	if err := h.Store.UpdateBaseEntity(id, entityUpdateOp(fields)); err != nil {
		return errgo.Notef(err, "cannot update base entity %q", id)
	}
	h.processEntries(entries)
	return nil
}

func (h *ReqHandler) updateEntity(id *router.ResolvedURL, fields map[string]interface{}, entries []audit.Entry) error {
	err := h.Store.UpdateEntity(id, entityUpdateOp(fields))
	if err != nil {
		return errgo.Notef(err, "cannot update %q", &id.URL)
	}
	err = h.Store.UpdateSearchFields(id, fields)
	if err != nil {
		return errgo.Notef(err, "cannot update %q", &id.URL)
	}
	h.processEntries(entries)
	return nil
}

// entityUpdateOp returns a mongo update operation that
// sets the given fields. Any nil fields will be unset.
func entityUpdateOp(fields map[string]interface{}) bson.D {
	setFields := make(bson.D, 0, len(fields))
	var unsetFields bson.D
	for name, val := range fields {
		if val != nil {
			setFields = append(setFields, bson.DocElem{name, val})
		} else {
			unsetFields = append(unsetFields, bson.DocElem{name, val})
		}
	}
	op := make(bson.D, 0, 2)
	if len(setFields) > 0 {
		op = append(op, bson.DocElem{"$set", setFields})
	}
	if len(unsetFields) > 0 {
		op = append(op, bson.DocElem{"$unset", unsetFields})
	}
	return op
}

func (h *ReqHandler) updateSearch(id *router.ResolvedURL, fields map[string]interface{}) error {
	return h.Store.UpdateSearch(id)
}

// updateSearchBase updates the search records for all entities with
// the same base URL as the given id.
func (h *ReqHandler) updateSearchBase(id *router.ResolvedURL, fields map[string]interface{}) error {
	baseURL := id.URL
	baseURL.Series = ""
	baseURL.Revision = -1
	if err := h.Store.UpdateSearchBaseURL(&baseURL); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func (h *ReqHandler) baseEntityQuery(id *router.ResolvedURL, fields map[string]int, req *http.Request) (interface{}, error) {
	val, err := h.Cache.BaseEntity(&id.URL, fields)
	if errgo.Cause(err) == params.ErrNotFound {
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "no matching charm or bundle for %s", id)
	}
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return val, nil
}

func (h *ReqHandler) entityQuery(id *router.ResolvedURL, selector map[string]int, req *http.Request) (interface{}, error) {
	val, err := h.Cache.Entity(&id.URL, selector)
	if errgo.Cause(err) == params.ErrNotFound {
		logger.Infof("entity %#v not found: %#v", id, err)
		return nil, errgo.WithCausef(nil, params.ErrNotFound, "no matching charm or bundle for %s", id)
	}
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return val, nil
}

var errNotImplemented = errgo.Newf("method not implemented")

// GET /debug
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-debug
func (h *ReqHandler) serveDebug(w http.ResponseWriter, req *http.Request) {
	router.WriteError(w, errNotImplemented)
}

// GET id/expand-id
// https://docs.google.com/a/canonical.com/document/d/1TgRA7jW_mmXoKH3JiwBbtPvQu7WiM6XMrz1wSrhTMXw/edit#bookmark=id.4xdnvxphb2si
func (h *ReqHandler) serveExpandId(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	baseURL := id.PreferredURL()
	baseURL.Revision = -1
	baseURL.Series = ""

	// baseURL now represents the base URL of the given id;
	// it will be a promulgated URL iff the original URL was
	// specified without a user, which will cause EntitiesQuery
	// to return entities that match appropriately.

	// Retrieve all the entities with the same base URL.
	q := h.Store.EntitiesQuery(baseURL).Select(bson.D{{"_id", 1}, {"promulgated-url", 1}})
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
	for _, doc := range docs {
		if err := h.AuthorizeEntityForOp(charmstore.EntityResolvedURL(doc), req, OpReadWithNoTerms); err != nil {
			continue
		}
		url := doc.PreferredURL(id.PromulgatedRevision != -1)
		response = append(response, params.ExpandedId{Id: url.String()})
	}

	// Write the response in JSON format.
	return httprequest.WriteJSON(w, http.StatusOK, response)
}

func badRequestf(underlying error, f string, a ...interface{}) error {
	err := errgo.WithCausef(underlying, params.ErrBadRequest, f, a...)
	err.(*errgo.Err).SetLocation(1)
	return err
}

// GET id/meta/charm-metadata
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetacharm-metadata
func (h *ReqHandler) metaCharmMetadata(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return entity.CharmMeta, nil
}

// GET id/meta/bundle-metadata
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetabundle-metadata
func (h *ReqHandler) metaBundleMetadata(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return entity.BundleData, nil
}

// GET id/meta/bundle-unit-count
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetabundle-unit-count
func (h *ReqHandler) metaBundleUnitCount(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return bundleCount(entity.BundleUnitCount), nil
}

// GET id/meta/bundle-machine-count
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetabundle-machine-count
func (h *ReqHandler) metaBundleMachineCount(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return bundleCount(entity.BundleMachineCount), nil
}

func bundleCount(x *int) interface{} {
	if x == nil {
		return nil
	}
	return params.BundleCount{
		Count: *x,
	}
}

// GET id/meta/manifest
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetamanifest
func (h *ReqHandler) metaManifest(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	r, size, err := h.Store.BlobStore.Open(entity.BlobName)
	if err != nil {
		return nil, errgo.Notef(err, "cannot open archive data for %s", id)
	}
	defer r.Close()
	zipReader, err := zip.NewReader(charmstore.ReaderAtSeeker(r), size)
	if err != nil {
		return nil, errgo.Notef(err, "cannot read archive data for %s", id)
	}
	// Collect the files.
	manifest := make([]params.ManifestFile, 0, len(zipReader.File))
	for _, file := range zipReader.File {
		fileInfo := file.FileInfo()
		if fileInfo.IsDir() {
			continue
		}
		manifest = append(manifest, params.ManifestFile{
			Name: file.Name,
			Size: fileInfo.Size(),
		})
	}
	return manifest, nil
}

// GET id/meta/charm-actions
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetacharm-actions
func (h *ReqHandler) metaCharmActions(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return entity.CharmActions, nil
}

// GET id/meta/charm-config
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetacharm-config
func (h *ReqHandler) metaCharmConfig(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return entity.CharmConfig, nil
}

// GET id/meta/terms
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaterms
func (h *ReqHandler) metaTerms(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	if entity.URL.Series == "bundle" {
		return nil, nil
	}
	if entity.CharmMeta == nil || len(entity.CharmMeta.Terms) == 0 {
		return []string{}, nil
	}
	return entity.CharmMeta.Terms, nil
}

// GET id/meta/color
func (h *ReqHandler) metaColor(id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return nil, errNotImplemented
}

// GET id/meta/archive-size
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaarchive-size
func (h *ReqHandler) metaArchiveSize(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return &params.ArchiveSizeResponse{
		Size: entity.Size,
	}, nil
}

// GET id/meta/hash
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetahash
func (h *ReqHandler) metaHash(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return &params.HashResponse{
		Sum: entity.BlobHash,
	}, nil
}

// GET id/meta/hash256
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetahash256
func (h *ReqHandler) metaHash256(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return &params.HashResponse{
		Sum: entity.BlobHash256,
	}, nil
}

// GET id/meta/tags
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetatags
func (h *ReqHandler) metaTags(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	var tags []string
	switch {
	case id.URL.Series == "bundle":
		tags = entity.BundleData.Tags
	case len(entity.CharmMeta.Tags) > 0:
		// TODO only return whitelisted tags.
		tags = entity.CharmMeta.Tags
	default:
		tags = entity.CharmMeta.Categories
	}
	return params.TagsResponse{
		Tags: tags,
	}, nil
}

// GET id/meta/stats/
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetastats
func (h *ReqHandler) metaStats(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {

	// Retrieve the aggregated downloads count for the specific revision.
	refresh, err := router.ParseBool(flags.Get("refresh"))
	if err != nil {
		return charmstore.SearchParams{}, badRequestf(err, "invalid refresh parameter")
	}
	counts, countsAllRevisions, err := h.Store.ArchiveDownloadCounts(id.PreferredURL(), refresh)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	// Return the response.
	return &params.StatsResponse{
		ArchiveDownloadCount: counts.Total,
		ArchiveDownload: params.StatsCount{
			Total: counts.Total,
			Day:   counts.LastDay,
			Week:  counts.LastWeek,
			Month: counts.LastMonth,
		},
		ArchiveDownloadAllRevisions: params.StatsCount{
			Total: countsAllRevisions.Total,
			Day:   countsAllRevisions.LastDay,
			Week:  countsAllRevisions.LastWeek,
			Month: countsAllRevisions.LastMonth,
		},
	}, nil
}

// GET id/meta/revision-info
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetarevision-info
func (h *ReqHandler) metaRevisionInfo(id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	searchURL := id.PreferredURL()
	searchURL.Revision = -1

	q := h.Store.EntitiesQuery(searchURL)
	if id.PromulgatedRevision != -1 {
		q = q.Sort("-promulgated-revision")
	} else {
		q = q.Sort("-revision")
	}
	var response params.RevisionInfoResponse
	iter := h.Cache.Iter(q, nil)
	for iter.Next() {
		e := iter.Entity()
		rurl := charmstore.EntityResolvedURL(e)
		if err := h.AuthorizeEntityForOp(rurl, req, OpReadWithNoTerms); err != nil {
			// We're not authorized to see the entity, so leave it out.
			// Note that the only time this will happen is when
			// the original URL is promulgated and has a development channel,
			// the charm has changed owners, and the old owner and
			// the new one have different dev ACLs. It's easiest
			// and most reliable just to check everything though.
			continue
		}
		if id.PromulgatedRevision != -1 {
			response.Revisions = append(response.Revisions, rurl.PromulgatedURL())
		} else {
			response.Revisions = append(response.Revisions, &rurl.URL)
		}
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "iteration failed")
	}
	return &response, nil
}

// GET id/meta/id-user
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaid-user
func (h *ReqHandler) metaIdUser(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return params.IdUserResponse{
		User: id.PreferredURL().User,
	}, nil
}

// GET id/meta/owner
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaowner
func (h *ReqHandler) metaOwner(_ *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return params.IdUserResponse{
		User: id.URL.User,
	}, nil
}

// GET id/meta/id-series
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaid-series
func (h *ReqHandler) metaIdSeries(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return params.IdSeriesResponse{
		Series: id.PreferredURL().Series,
	}, nil
}

// GET id/meta/id
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaid
func (h *ReqHandler) metaId(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	u := id.PreferredURL()
	return params.IdResponse{
		Id:       u,
		User:     u.User,
		Series:   u.Series,
		Name:     u.Name,
		Revision: u.Revision,
	}, nil
}

// GET id/meta/id-name
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaid-name
func (h *ReqHandler) metaIdName(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return params.IdNameResponse{
		Name: id.URL.Name,
	}, nil
}

// GET id/meta/id-revision
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaid-revision
func (h *ReqHandler) metaIdRevision(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return params.IdRevisionResponse{
		Revision: id.PreferredURL().Revision,
	}, nil
}

// GET id/meta/supported-series
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetasupported-series
func (h *ReqHandler) metaSupportedSeries(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	if entity.URL.Series == "bundle" {
		return nil, nil
	}
	return &params.SupportedSeriesResponse{
		SupportedSeries: entity.SupportedSeries,
	}, nil
}

// GET id/meta/extra-info
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaextra-info
func (h *ReqHandler) metaExtraInfo(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	// The extra-info is stored in mongo as simple byte
	// slices, so convert the values to json.RawMessages
	// so that the client will see the original JSON.
	m := make(map[string]*json.RawMessage)
	for key, val := range entity.ExtraInfo {
		jmsg := json.RawMessage(val)
		m[key] = &jmsg
	}
	return m, nil
}

// GET id/meta/extra-info/key
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaextra-infokey
func (h *ReqHandler) metaExtraInfoWithKey(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	path = strings.TrimPrefix(path, "/")
	var data json.RawMessage = entity.ExtraInfo[path]
	if len(data) == 0 {
		return nil, nil
	}
	return &data, nil
}

// PUT id/meta/extra-info
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#put-idmetaextra-info
func (h *ReqHandler) putMetaExtraInfo(id *router.ResolvedURL, path string, val *json.RawMessage, updater *router.FieldUpdater, req *http.Request) error {
	var fields map[string]*json.RawMessage
	if err := json.Unmarshal(*val, &fields); err != nil {
		return errgo.Notef(err, "cannot unmarshal extra-info body")
	}
	// Check all the fields are OK before adding any fields to be updated.
	for key := range fields {
		if err := checkExtraInfoKey(key, "extra-info"); err != nil {
			return err
		}
	}
	for key, val := range fields {
		if val == nil {
			updater.UpdateField("extrainfo."+key, nil, nil)
		} else {
			updater.UpdateField("extrainfo."+key, *val, nil)
		}
	}
	return nil
}

var nullBytes = []byte("null")

// PUT id/meta/extra-info/key
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#put-idmetaextra-infokey
func (h *ReqHandler) putMetaExtraInfoWithKey(id *router.ResolvedURL, path string, val *json.RawMessage, updater *router.FieldUpdater, req *http.Request) error {
	key := strings.TrimPrefix(path, "/")
	if err := checkExtraInfoKey(key, "extra-info"); err != nil {
		return err
	}
	// If the user puts null, we treat that as if they want to
	// delete the field.
	if val == nil || bytes.Equal(*val, nullBytes) {
		updater.UpdateField("extrainfo."+key, nil, nil)
	} else {
		updater.UpdateField("extrainfo."+key, *val, nil)
	}
	return nil
}

// GET id/meta/common-info
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetacommon-info
func (h *ReqHandler) metaCommonInfo(entity *mongodoc.BaseEntity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	// The common-info is stored in mongo as simple byte
	// slices, so convert the values to json.RawMessages
	// so that the client will see the original JSON.
	m := make(map[string]*json.RawMessage)
	for key, val := range entity.CommonInfo {
		jmsg := json.RawMessage(val)
		m[key] = &jmsg
	}
	return m, nil
}

// GET id/meta/common-info/key
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetacommon-infokey
func (h *ReqHandler) metaCommonInfoWithKey(entity *mongodoc.BaseEntity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	path = strings.TrimPrefix(path, "/")
	var data json.RawMessage = entity.CommonInfo[path]
	if len(data) == 0 {
		return nil, nil
	}
	return &data, nil
}

// PUT id/meta/common-info
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#put-idmetacommon-info
func (h *ReqHandler) putMetaCommonInfo(id *router.ResolvedURL, path string, val *json.RawMessage, updater *router.FieldUpdater, req *http.Request) error {
	var fields map[string]*json.RawMessage
	if err := json.Unmarshal(*val, &fields); err != nil {
		return errgo.Notef(err, "cannot unmarshal common-info body")
	}
	// Check all the fields are OK before adding any fields to be updated.
	for key := range fields {
		if err := checkExtraInfoKey(key, "common-info"); err != nil {
			return err
		}
	}
	for key, val := range fields {
		if val == nil {
			updater.UpdateField("commoninfo."+key, nil, nil)
		} else {
			updater.UpdateField("commoninfo."+key, *val, nil)
		}
	}
	return nil
}

// PUT id/meta/common-info/key
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#put-idmetacommon-infokey
func (h *ReqHandler) putMetaCommonInfoWithKey(id *router.ResolvedURL, path string, val *json.RawMessage, updater *router.FieldUpdater, req *http.Request) error {
	key := strings.TrimPrefix(path, "/")
	if err := checkExtraInfoKey(key, "common-info"); err != nil {
		return err
	}
	// If the user puts null, we treat that as if they want to
	// delete the field.
	if val == nil || bytes.Equal(*val, nullBytes) {
		updater.UpdateField("commoninfo."+key, nil, nil)
	} else {
		updater.UpdateField("commoninfo."+key, *val, nil)
	}
	return nil
}

func checkExtraInfoKey(key string, field string) error {
	if strings.ContainsAny(key, "./$") {
		return errgo.WithCausef(nil, params.ErrBadRequest, "bad key for "+field)
	}
	return nil
}

// GET id/meta/perm
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaperm
func (h *ReqHandler) metaPerm(entity *mongodoc.BaseEntity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	ch, err := h.entityChannel(id)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	acls := entity.ChannelACLs[ch]
	return params.PermResponse{
		Read:  acls.Read,
		Write: acls.Write,
	}, nil
}

// PUT id/meta/perm
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#put-idmeta
func (h *ReqHandler) putMetaPerm(id *router.ResolvedURL, path string, val *json.RawMessage, updater *router.FieldUpdater, req *http.Request) error {
	var perms params.PermRequest
	if err := json.Unmarshal(*val, &perms); err != nil {
		return errgo.Mask(err)
	}
	ch, err := h.entityChannel(id)
	if err != nil {
		return errgo.Mask(err)
	}
	// TODO use only one UpdateField operation?
	updater.UpdateField(string("channelacls."+ch+".read"), perms.Read, &audit.Entry{
		Op:     audit.OpSetPerm,
		Entity: &id.URL,
		ACL: &audit.ACL{
			Read: perms.Read,
		},
	})
	updater.UpdateField(string("channelacls."+ch+".write"), perms.Write, &audit.Entry{
		Op:     audit.OpSetPerm,
		Entity: &id.URL,
		ACL: &audit.ACL{
			Write: perms.Write,
		},
	})
	updater.UpdateSearch()
	return nil
}

// GET id/meta/promulgated
// See https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetapromulgated
func (h *ReqHandler) metaPromulgated(entity *mongodoc.BaseEntity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return params.PromulgatedResponse{
		Promulgated: bool(entity.Promulgated),
	}, nil
}

// GET id/meta/can-ingest
// See https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetacan-ingest
func (h *ReqHandler) metaCanIngest(entity *mongodoc.BaseEntity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return params.CanIngestResponse{
		CanIngest: !entity.NoIngest,
	}, nil
}

// GET id/meta/perm/key
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetapermkey
func (h *ReqHandler) metaPermWithKey(entity *mongodoc.BaseEntity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	ch, err := h.entityChannel(id)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	acls := entity.ChannelACLs[ch]
	switch path {
	case "/read":
		return acls.Read, nil
	case "/write":
		return acls.Write, nil
	}
	return nil, errgo.WithCausef(nil, params.ErrNotFound, "unknown permission")
}

// PUT id/meta/perm/key
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#put-idmetapermkey
func (h *ReqHandler) putMetaPermWithKey(id *router.ResolvedURL, path string, val *json.RawMessage, updater *router.FieldUpdater, req *http.Request) error {
	ch, err := h.entityChannel(id)
	if err != nil {
		return errgo.Mask(err)
	}
	var perms []string
	if err := json.Unmarshal(*val, &perms); err != nil {
		return errgo.Mask(err)
	}
	switch path {
	case "/read":
		updater.UpdateField(string("channelacls."+ch+".read"), perms, &audit.Entry{
			Op:     audit.OpSetPerm,
			Entity: &id.URL,
			ACL: &audit.ACL{
				Read: perms,
			},
		})
		updater.UpdateSearch()
		return nil
	case "/write":
		updater.UpdateField(string("channelacls."+ch+".write"), perms, &audit.Entry{
			Op:     audit.OpSetPerm,
			Entity: &id.URL,
			ACL: &audit.ACL{
				Write: perms,
			},
		})
		return nil
	}
	return errgo.WithCausef(nil, params.ErrNotFound, "unknown permission")
}

// GET id/meta/published
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetapublished
func (h *ReqHandler) metaPublished(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	baseEntity, err := h.Cache.BaseEntity(entity.URL, charmstore.FieldSelector("channelentities"))
	if err != nil {
		return nil, errgo.Mask(err)
	}
	info := make([]params.PublishedInfo, 0, 2)
	if entity.Development {
		info = append(info, params.PublishedInfo{
			Channel: params.DevelopmentChannel,
		})
	}
	if entity.Stable {
		info = append(info, params.PublishedInfo{
			Channel: params.StableChannel,
		})
	}
	for i, pinfo := range info {
		// The entity is current for a channel if any series within
		// a channel refers to the entity.
		for _, url := range baseEntity.ChannelEntities[pinfo.Channel] {
			if *url == *entity.URL {
				info[i].Current = true
			}
		}
	}
	return &params.PublishedResponse{
		Info: info,
	}, nil
}

// GET id/meta/archive-upload-time
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-idmetaarchive-upload-time
func (h *ReqHandler) metaArchiveUploadTime(entity *mongodoc.Entity, id *router.ResolvedURL, path string, flags url.Values, req *http.Request) (interface{}, error) {
	return &params.ArchiveUploadTimeResponse{
		UploadTime: entity.UploadTime.UTC(),
	}, nil
}

// GET changes/published[?limit=$count][&start=$fromdate][&stop=$todate]
// https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-changespublished
func (h *ReqHandler) serveChangesPublished(_ http.Header, r *http.Request) (interface{}, error) {
	start, stop, err := parseDateRange(r.Form)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	limit := -1
	if limitStr := r.Form.Get("limit"); limitStr != "" {
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			return nil, badRequestf(nil, "invalid 'limit' value")
		}
	}
	var tquery bson.D
	if !start.IsZero() {
		tquery = make(bson.D, 0, 2)
		tquery = append(tquery, bson.DocElem{
			Name:  "$gte",
			Value: start,
		})
	}
	if !stop.IsZero() {
		tquery = append(tquery, bson.DocElem{
			Name:  "$lte",
			Value: stop,
		})
	}
	var findQuery bson.D
	if len(tquery) > 0 {
		findQuery = bson.D{{"uploadtime", tquery}}
	}
	query := h.Store.DB.Entities().
		Find(findQuery).
		Sort("-uploadtime")
	iter := h.Cache.Iter(query, charmstore.FieldSelector("uploadtime"))

	results := []params.Published{}
	var count int
	for iter.Next() {
		entity := iter.Entity()
		// Ignore entities that aren't readable by the current user.
		if err := h.AuthorizeEntityForOp(charmstore.EntityResolvedURL(entity), r, OpReadWithNoTerms); err != nil {
			continue
		}
		results = append(results, params.Published{
			Id:          entity.URL,
			PublishTime: entity.UploadTime.UTC(),
		})
		count++
		if limit > 0 && limit <= count {
			iter.Close()
			break
		}
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Mask(err)
	}
	return results, nil
}

// GET /macaroon
// See https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-macaroon
// Return a macaroon that will enable access to that can be checked by just
// knowing the user's name.
func (h *ReqHandler) serveMacaroon(_ http.Header, _ *http.Request) (interface{}, error) {
	return h.newMacaroon(authnCheckableOps, nil, nil, false)
}

// GET /delegatable-macaroon
// See https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#get-delegatable-macaroon
func (h *ReqHandler) serveDelegatableMacaroon(_ http.Header, req *http.Request) (interface{}, error) {
	values, err := url.ParseQuery(req.URL.RawQuery)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	idStrs := values["id"]
	// No entity ids, so we provide a macaroon that's good for any entity that the
	// user can access, as long as that entity doesn't have terms and conditions.
	if len(idStrs) == 0 {
		// Note that we require authorization even though we allow
		// anyone to obtain a delegatable macaroon. This means
		// that we will be able to add the declared caveats to
		// the returned macaroon.
		auth, err := h.Authenticate(req)
		if err != nil {
			return nil, errgo.Mask(err, errgo.Any)
		}
		if auth.Username == "" {
			return nil, errgo.WithCausef(nil, params.ErrForbidden, "delegatable macaroon is not obtainable using admin credentials")
		}
		// TODO propagate expiry time from macaroons in request.
		m, err := h.Store.Bakery.NewMacaroon("", nil, []checkers.Caveat{
			checkers.DeclaredCaveat(UsernameAttr, auth.Username),
			checkers.TimeBeforeCaveat(time.Now().Add(DelegatableMacaroonExpiry)),
			checkers.AllowCaveat(authnCheckableOps...),
		})
		if err != nil {
			return nil, errgo.Mask(err)
		}
		return m, nil
	}
	urls := make([]*charm.URL, len(idStrs))
	for i, idStr := range idStrs {
		u, err := charm.ParseURL(idStr)
		if err != nil {
			return nil, errgo.WithCausef(err, params.ErrBadRequest, `bad "id" parameter %q`, idStr)
		}
		urls[i] = u
	}
	ids, err := h.ResolveURLs(urls)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	// ResolveURLs will return nil elements for any ids that aren't found.
	for i, id := range ids {
		if id == nil {
			return nil, errgo.WithCausef(err, params.ErrNotFound, "%q not found", idStrs[i])
		}
	}

	auth, err := h.authorize(authorizeParams{
		req:       req,
		entityIds: ids,
		// Note: we don't allow OpWrite here even though we could, because
		// the current use-case for delegatable macaroon doesn't require it.
		ops: []string{OpReadWithTerms, OpReadWithNoTerms},
	})
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	if auth.Username == "" {
		if !auth.Admin {
			return nil, errgo.WithCausef(nil, params.ErrForbidden, "delegatable macaroon cannot be obtained for public entities")
		}
		return nil, errgo.WithCausef(nil, params.ErrForbidden, "delegatable macaroon is not obtainable using admin credentials (admin %v)", auth.Admin)
	}

	// After this time, clients will be forced to renew the macaroon, even
	// though it remains technically valid.
	activeExpireTime := time.Now().Add(DelegatableMacaroonExpiry)

	// TODO propagate expiry time from macaroons in request.
	m, err := h.Store.Bakery.NewMacaroon("", nil, []checkers.Caveat{
		checkers.DeclaredCaveat(UsernameAttr, auth.Username),
		isEntityCaveat(ids),
		activeTimeBeforeCaveat(activeExpireTime),
	})
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return m, nil
}

// GET /whoami
// See https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#whoami
func (h *ReqHandler) serveWhoAmI(_ http.Header, req *http.Request) (interface{}, error) {
	auth, err := h.Authenticate(req)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	if auth.Admin {
		return nil, errgo.WithCausef(nil, params.ErrForbidden, "admin credentials used")
	}
	groups, err := h.Handler.GroupsForUser(auth.Username)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	return params.WhoAmIResponse{
		User:   auth.Username,
		Groups: groups,
	}, nil
}

// PUT id/promulgate
// See https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#put-idpromulgate
func (h *ReqHandler) servePromulgate(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	// Note: the promulgator must be in the promulgators group but
	// doesn't need write access to the entity.
	if _, err := h.authorize(authorizeParams{
		req: req,
		acls: []mongodoc.ACL{{
			Write: []string{PromulgatorsGroup},
		}},
		ops:              []string{OpWrite},
		entityIds:        []*router.ResolvedURL{id},
		ignoreEntityACLs: true,
	}); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	if req.Method != "PUT" {
		return errgo.WithCausef(nil, params.ErrMethodNotAllowed, "%s not allowed", req.Method)
	}
	data, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return errgo.Mask(err)
	}
	var promulgate params.PromulgateRequest
	if err := json.Unmarshal(data, &promulgate); err != nil {
		return errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	if err := h.Store.SetPromulgated(id, promulgate.Promulgated); err != nil {
		return errgo.Mask(err, errgo.Any)
	}

	if promulgate.Promulgated {
		// Set write permissions to promulgators only, so that
		// the user cannot just publish newer promulgated
		// versions of the charm or bundle. Promulgators are
		// responsible of reviewing and publishing subsequent
		// revisions of this entity.
		if err := h.updateBaseEntity(id, map[string]interface{}{
			"channelacls.stable.write": []string{PromulgatorsGroup},
		}, nil); err != nil {
			return errgo.Notef(err, "cannot set permissions for %q", id)
		}
	}

	// Build an audit entry for this promulgation.
	e := audit.Entry{
		Entity: &id.URL,
	}
	if promulgate.Promulgated {
		e.Op = audit.OpPromulgate
	} else {
		e.Op = audit.OpUnpromulgate
	}
	h.addAudit(e)

	return nil
}

// validPublishChannels holds the set of channels that can
// be the target of a publish request.
var validPublishChannels = map[params.Channel]bool{
	params.DevelopmentChannel: true,
	params.StableChannel:      true,
}

// PUT id/publish
// See https://github.com/juju/charmstore/blob/v5-unstable/docs/API.md#put-idpublish
func (h *ReqHandler) servePublish(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
	// Perform basic validation of the request.
	if req.Method != "PUT" {
		return errgo.WithCausef(nil, params.ErrMethodNotAllowed, "%s not allowed", req.Method)
	}

	// Retrieve the requested action from the request body.
	var publish struct {
		params.PublishRequest `httprequest:",body"`
	}
	if err := httprequest.Unmarshal(httprequest.Params{Request: req}, &publish); err != nil {
		return badRequestf(err, "cannot unmarshal publish request body")
	}
	chans := publish.Channels
	if len(chans) == 0 {
		return badRequestf(nil, "no channels provided")
	}
	for _, c := range chans {
		if !validPublishChannels[c] {
			return badRequestf(nil, "cannot publish to %q", c)
		}
	}

	// Retrieve the base entity so that we can check permissions.
	baseEntity, err := h.Cache.BaseEntity(&id.URL, charmstore.FieldSelector("channelacls"))
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}

	// Authorize the operation. Users must have write permissions on the ACLs
	// on all the channels being published to.
	acls := make([]mongodoc.ACL, 0, len(chans))
	for _, c := range chans {
		acls = append(acls, baseEntity.ChannelACLs[c])
	}
	if _, err := h.authorize(authorizeParams{
		req:              req,
		acls:             acls,
		entityIds:        []*router.ResolvedURL{id},
		ignoreEntityACLs: true, // acls holds all the ACLs we care about.
		ops:              []string{OpWrite},
	}); err != nil {
		return errgo.Mask(err, errgo.Any)
	}

	if err := h.Store.Publish(id, publish.Resources, chans...); err != nil {
		if errgo.Cause(err) == charmstore.ErrPublishResourceMismatch {
			return errgo.WithCausef(err, params.ErrBadRequest, "")
		}
		return errgo.NoteMask(err, "cannot publish charm or bundle", errgo.Is(params.ErrNotFound))
	}
	// TODO add publish audit
	return nil
}

// serveSetAuthCookie sets the provided macaroon slice as a cookie on the
// client.
func (h *ReqHandler) serveSetAuthCookie(w http.ResponseWriter, req *http.Request) error {
	// Allow cross-domain requests for the origin of this specific request so
	// that cookies can be set even if the request is xhr.
	w.Header().Set("Access-Control-Allow-Origin", req.Header.Get("Origin"))
	if req.Method != "PUT" {
		return errgo.WithCausef(nil, params.ErrMethodNotAllowed, "%s not allowed", req.Method)
	}
	var p params.SetAuthCookie
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&p); err != nil {
		return errgo.Notef(err, "cannot unmarshal macaroons")
	}
	cookie, err := httpbakery.NewCookie(p.Macaroons)
	if err != nil {
		return errgo.Notef(err, "cannot create macaroons cookie")
	}
	cookie.Path = "/"
	cookie.Name = "macaroon-ui"
	http.SetCookie(w, cookie)
	return nil
}

// ResolvedIdHandler represents a HTTP handler that is invoked
// on a resolved entity id.
type ResolvedIdHandler func(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error

// AuthIdHandler returns a ResolvedIdHandler that uses h.Router.Context.AuthorizeEntity to
// check that the client is authorized to perform the HTTP request method before
// invoking f.
//
// Note that it only accesses h.Router.Context when the returned
// handler is called.
func (h *ReqHandler) AuthIdHandler(f ResolvedIdHandler) ResolvedIdHandler {
	return func(id *router.ResolvedURL, w http.ResponseWriter, req *http.Request) error {
		if err := h.Router.Context.AuthorizeEntity(id, req); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
		if err := f(id, w, req); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
		return nil
	}
}

// ResolvedIdHandler returns an id handler that uses h.Router.Context.ResolveURL
// to resolves any entity ids before calling f with the resolved id.
//
// Any specified fields will be added to the fields required by the cache, so
// they will be pre-fetched by ResolveURL.
//
// Note that it only accesses h.Router.Context when the returned
// handler is called.
func (h *ReqHandler) ResolvedIdHandler(f ResolvedIdHandler, cacheFields ...string) router.IdHandler {
	fields := charmstore.FieldSelector(cacheFields...)
	return func(id *charm.URL, w http.ResponseWriter, req *http.Request) error {
		h.Cache.AddEntityFields(fields)
		rid, err := h.Router.Context.ResolveURL(id)
		if err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		return f(rid, w, req)
	}
}

var testAddAuditCallback func(e audit.Entry)

// addAudit delegates an audit entry to the store to record an audit log after
// it has set correctly the user doing the action.
func (h *ReqHandler) addAudit(e audit.Entry) {
	if h.auth.Username == "" && !h.auth.Admin {
		panic("No auth set in ReqHandler")
	}
	e.User = h.auth.Username
	if h.auth.Admin && e.User == "" {
		e.User = "admin"
	}
	h.Store.AddAudit(e)
	if testAddAuditCallback != nil {
		testAddAuditCallback(e)
	}
}

// logout handles the GET /v5/logout endpoint that is used to log out of
// charmstore.
func logout(w http.ResponseWriter, r *http.Request) {
	for _, c := range r.Cookies() {
		if !strings.HasPrefix(c.Name, "macaroon-") {
			continue
		}
		c.Value = ""
		c.MaxAge = -1
		c.Path = "/"
		http.SetCookie(w, c)
	}
}
