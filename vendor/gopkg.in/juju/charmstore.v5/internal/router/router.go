// Copyright 2014-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The router package implements an HTTP request router for charm store
// HTTP requests.
package router // import "gopkg.in/juju/charmstore.v5/internal/router"

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/juju/utils/parallel"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"
	charm "gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"

	"gopkg.in/juju/charmstore.v5/internal/monitoring"
	"gopkg.in/juju/charmstore.v5/internal/series"
)

// Implementation note on error handling:
//
// We use errgo.Any only when necessary, so that we can see at a glance
// which are the possible places that could be returning an error with a
// Cause (the only kind of error that can end up setting an HTTP status
// code)

// BulkIncludeHandler represents a metadata handler that can
// handle multiple metadata "include" requests in a single batch.
//
// For simple metadata handlers that cannot be
// efficiently combined, see SingleIncludeHandler.
//
// All handlers may assume that http.Request.ParseForm
// has been called to parse the URL form values.
type BulkIncludeHandler interface {
	// Key returns a value that will be used to group handlers
	// together in preparation for a call to HandleGet or HandlePut.
	// The key should be comparable for equality.
	// Please do not return NaN. That would be silly, OK?
	Key() interface{}

	// HandleGet returns the results of invoking all the given handlers
	// on the given charm or bundle id. Each result is held in
	// the respective element of the returned slice.
	//
	// All of the handlers' Keys will be equal to the receiving handler's
	// Key.
	//
	// Each item in paths holds the remaining metadata path
	// for the handler in the corresponding position
	// in hs after the prefix in Handlers.Meta has been stripped,
	// and flags holds all the URL query values.
	//
	// TODO(rog) document indexed errors.
	HandleGet(hs []BulkIncludeHandler, id *ResolvedURL, paths []string, flags url.Values, req *http.Request) ([]interface{}, error)

	// HandlePut invokes a PUT request on all the given handlers on
	// the given charm or bundle id. If there is an error, the
	// returned errors slice should contain one element for each element
	// in paths. The error for handler hs[i] should be returned in errors[i].
	// If there is no error, an empty slice should be returned.
	//
	// Each item in paths holds the remaining metadata path
	// for the handler in the corresponding position
	// in hs after the prefix in Handlers.Meta has been stripped,
	// and flags holds all the url query values.
	HandlePut(hs []BulkIncludeHandler, id *ResolvedURL, paths []string, values []*json.RawMessage, req *http.Request) []error
}

// IdHandler handles a charm store request rooted at the given id.
// The request path (req.URL.Path) holds the URL path after
// the id has been stripped off.
type IdHandler func(charmId *charm.URL, w http.ResponseWriter, req *http.Request) error

// Handlers specifies how HTTP requests will be routed
// by the router. All errors returned by the handlers will
// be processed by WriteError with their Cause left intact.
// This means that, for example, if they return an error
// with a Cause that is params.ErrNotFound, the HTTP
// status code will reflect that (assuming the error has
// not been absorbed by the bulk metadata logic).
type Handlers struct {
	// Global holds handlers for paths not matched by Meta or Id.
	// The map key is the path; the value is the handler that will
	// be used to handle that path.
	//
	// Path matching is by matched by longest-prefix - the same as
	// http.ServeMux.
	//
	// Note that, unlike http.ServeMux, the prefix is stripped
	// from the URL path before the hander is invoked,
	// matching the behaviour of the other handlers.
	Global map[string]http.Handler

	// Id holds handlers for paths which correspond to a single
	// charm or bundle id other than the meta path. The map key
	// holds the first element of the path, which may end in a
	// trailing slash (/) to indicate that longer paths are allowed
	// too.
	Id map[string]IdHandler

	// Meta holds metadata handlers for paths under the meta
	// endpoint. The map key holds the first element of the path,
	// which may end in a trailing slash (/) to indicate that longer
	// paths are allowed too.
	Meta map[string]BulkIncludeHandler
}

// Router represents a charm store HTTP request router.
type Router struct {
	// Context holds context that the router was created with.
	Context Context

	handlers *Handlers
	handler  http.Handler

	// monitor holds a metric monitor to time a request.
	Monitor monitoring.Request
}

// ResolvedURL represents a URL that has been resolved by resolveURL.
type ResolvedURL struct {
	// URL holds the canonical URL for the entity, as used as a key into
	// the Entities collection. URL.User should always be non-empty
	// and URL.Revision should never be -1. URL.Series will only be non-empty
	// if the URL refers to a multi-series charm.
	URL charm.URL

	// PreferredSeries holds the series to return in PreferredURL
	// if URL itself contains no series.
	PreferredSeries string

	// PromulgatedRevision holds the revision of the promulgated version of the
	// charm or -1 if the corresponding entity is not promulgated.
	PromulgatedRevision int
}

// MustNewResolvedURL returns a new ResolvedURL by parsing
// the entity URL in urlStr. The promulgatedRev parameter
// specifies the value of PromulgatedRevision in the returned
// value.
//
// This function panics if urlStr cannot be parsed as a charm.URL
// or if it is not fully specified, including user and revision.
func MustNewResolvedURL(urlStr string, promulgatedRev int) *ResolvedURL {
	url := mustParseURL(urlStr)
	if url.User == "" || url.Revision == -1 {
		panic(fmt.Errorf("incomplete url %v", urlStr))
	}
	return &ResolvedURL{
		URL:                 *url,
		PromulgatedRevision: promulgatedRev,
	}
}

// PreferredURL returns the promulgated URL for the given id if there is
// one, otherwise it returns the non-promulgated URL. The returned
// *charm.URL may be modified freely.
//
// If id.PreferredSeries is non-empty, the returns charm URL
// will always have a non-empty series.
func (id *ResolvedURL) PreferredURL() *charm.URL {
	u := id.URL
	if u.Series == "" && id.PreferredSeries != "" {
		u.Series = id.PreferredSeries
	}
	if id.PromulgatedRevision == -1 {
		return &u
	}
	u.User = ""
	u.Revision = id.PromulgatedRevision
	return &u
}

// PromulgatedURL returns the promulgated URL for id if there
// is one, or nil otherwise.
func (id *ResolvedURL) PromulgatedURL() *charm.URL {
	if id.PromulgatedRevision == -1 {
		return nil
	}
	u := id.URL
	u.User = ""
	u.Revision = id.PromulgatedRevision
	return &u
}

func (id *ResolvedURL) GoString() string {
	// Make the URL member visible as a string
	// rather than as a set of members.
	var gid = struct {
		URL                 string
		PreferredSeries     string
		PromulgatedRevision int
	}{
		URL:                 id.URL.String(),
		PreferredSeries:     id.PreferredSeries,
		PromulgatedRevision: id.PromulgatedRevision,
	}
	return fmt.Sprintf("%#v", gid)
}

// String returns the preferred string representation of u.
// It prefers to use the promulgated URL when there is one.
func (u *ResolvedURL) String() string {
	return u.PreferredURL().String()
}

// Context provides contextual information for a router.
type Context interface {
	// ResolveURL will be called to resolve ids in
	// router paths - it should return the fully
	// resolved URL corresponding to the given id.
	// If the entity referred to by the URL does not
	// exist, it should return an error with a params.ErrNotFound
	// cause.
	ResolveURL(id *charm.URL) (*ResolvedURL, error)

	// ResolveURLs is like ResolveURL but resolves multiple URLs
	// at the same time. The length of the returned slice should
	// be len(ids); any entities that are not found should be represented
	// by nil elements.
	ResolveURLs(ids []*charm.URL) ([]*ResolvedURL, error)

	// AuthorizeEntity will be called to authorize requests
	// to any BulkIncludeHandlers. All other handlers are expected
	// to handle their own authorization.
	AuthorizeEntity(id *ResolvedURL, req *http.Request) error

	// WillIncludeMetadata informs the context that the given metadata
	// includes will be required in the request. This allows the context
	// to prime any cache fetches to fetch this data when early
	// fetches which may not require the metadata are made.
	// This method should ignore any unrecognized names.
	WillIncludeMetadata(includes []string)
}

// New returns a charm store router that will route requests to
// the given handlers and retrieve metadata from the given database.
//
// The Context argument provides additional context to the
// router. Any errors returned by the context methods will
// have their cause preserved when creating the error return
// as for the handlers.
func New(
	handlers *Handlers,
	ctxt Context,
) *Router {
	r := &Router{
		handlers: handlers,
		Context:  ctxt,
	}
	mux := NewServeMux()
	mux.Handle("/meta/", http.StripPrefix("/meta", HandleErrors(r.serveBulkMeta)))
	for path, handler := range r.handlers.Global {
		path = "/" + path
		prefix := strings.TrimSuffix(path, "/")
		handler := handler
		mux.Handle(path, http.StripPrefix(prefix, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			r.Monitor.SetKind(path[1:])
			handler.ServeHTTP(w, req)
		})))
	}
	mux.Handle("/", HandleErrors(r.serveIds))
	r.handler = mux
	return r
}

// ServeHTTP implements http.Handler.ServeHTTP.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Allow cross-domain access from anywhere, including AJAX
	// requests. An AJAX request will add an X-Requested-With:
	// XMLHttpRequest header, which is a non-standard header, and
	// hence will require a pre-flight request, so we need to
	// specify that that header is allowed, and we also need to
	// implement the OPTIONS method so that the pre-flight request
	// can work.
	// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Access_control_CORS
	header := w.Header()
	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Access-Control-Allow-Headers", "Bakery-Protocol-Version, Macaroons, X-Requested-With")
	header.Set("Access-Control-Allow-Credentials", "true")
	header.Set("Access-Control-Cache-Max-Age", "600")
	header.Set("Access-Control-Allow-Methods", "DELETE,GET,HEAD,PUT,POST,OPTIONS")
	header.Set("Access-Control-Expose-Headers", "WWW-Authenticate")

	if req.Method == "OPTIONS" {
		// We cheat here and say that all methods are allowed,
		// even though any individual endpoint will allow
		// only a subset of these. This means we can avoid
		// putting OPTIONS handling in every endpoint,
		// and it shouldn't actually matter in practice.
		header.Set("Allow", "DELETE,GET,HEAD,PUT,POST")
		header.Set("Access-Control-Allow-Origin", req.Header.Get("Origin"))
		return
	}
	if err := req.ParseForm(); err != nil {
		WriteError(context.TODO(), w, errgo.Notef(err, "cannot parse form"))
		return
	}
	r.handler.ServeHTTP(w, req)
}

// Handlers returns the set of handlers that the router was created with.
// This should not be changed.
func (r *Router) Handlers() *Handlers {
	return r.handlers
}

// serveIds serves requests that may be rooted at a charm or bundle id.
func (r *Router) serveIds(w http.ResponseWriter, req *http.Request) error {
	// We can ignore a trailing / because we do not return any
	// relative URLs. If we start to return relative URL redirects,
	// we will need to redirect non-slash-terminated URLs
	// to slash-terminated URLs.
	// http://cdivilly.wordpress.com/2014/03/11/why-trailing-slashes-on-uris-are-important/
	path := strings.TrimSuffix(req.URL.Path, "/")
	url, path, err := splitId(path)
	if err != nil {
		return errgo.WithCausef(err, params.ErrNotFound, "")
	}
	key, path := handlerKey(path)
	if key == "" {
		return errgo.WithCausef(nil, params.ErrNotFound, "")
	}
	handler := r.handlers.Id[key]
	if handler != nil {
		r.Monitor.SetKind(key)
		req.URL.Path = path
		err := handler(url, w, req)
		// Note: preserve error cause from handlers.
		return errgo.Mask(err, errgo.Any)
	}
	if key != "meta/" && key != "meta" {
		return errgo.WithCausef(nil, params.ErrNotFound, params.ErrNotFound.Error())
	}
	req.URL.Path = path
	return r.serveMeta(url, w, req)
}

func idHandlerNeedsResolveURL(req *http.Request) bool {
	return req.Method != "POST" && req.Method != "PUT"
}

// handlerKey returns a key that can be used to look up a handler at the
// given path, and the remaining path elements. If there is no possible
// key, the returned key is empty.
func handlerKey(path string) (key, rest string) {
	path = strings.TrimPrefix(path, "/")
	key, i := splitPath(path, 0)
	if key == "" {
		// TODO what *should* we get if we GET just an id?
		return "", rest
	}
	if i < len(path)-1 {
		// There are more elements, so include the / character
		// that terminates the element.
		return path[0 : i+1], path[i:]
	}
	return key, ""
}

func (r *Router) serveMeta(id *charm.URL, w http.ResponseWriter, req *http.Request) error {
	switch req.Method {
	case "GET", "HEAD":
		r.willIncludeMetadata(req)
		rurl, err := r.Context.ResolveURL(id)
		if err != nil {
			// Note: preserve error cause from ResolveURL.
			return errgo.Mask(err, errgo.Any)
		}
		resp, err := r.serveMetaGet(rurl, req)
		if err != nil {
			// Note: preserve error causes from meta handlers.
			return errgo.Mask(err, errgo.Any)
		}
		httprequest.WriteJSON(w, http.StatusOK, resp)
		return nil
	case "PUT":
		rurl, err := r.Context.ResolveURL(id)
		if err != nil {
			// Note: preserve error cause from ResolveURL.
			return errgo.Mask(err, errgo.Any)
		}
		// Put requests don't return any data unless there's
		// an error.
		return r.serveMetaPut(rurl, req)
	}
	return params.ErrMethodNotAllowed
}

// willIncludeMetadata notifies the context about any metadata
// that will probably be required by the request, so that initial
// fetches (for example by ResolveURL) can fetch additional
// data too. The request is assumed to be for a /meta request,
// with the actual meta path in req.Path (e.g. /any, /metaname).
func (r *Router) willIncludeMetadata(req *http.Request) {
	// We assume that any "include" attribute is an included metadata
	// specifier. This is perhaps arguable, but it's currently true,
	// including more fields can't do any harm and it's a simple rule.
	// Note that we must call this method before resolving the URL.
	includes := req.Form["include"]
	if path := strings.TrimPrefix(req.URL.Path, "/"); path != "" && path != "any" {
		includes = append(includes, path)
	}
	r.Context.WillIncludeMetadata(includes)
}

func (r *Router) serveMetaGet(rurl *ResolvedURL, req *http.Request) (interface{}, error) {
	// TODO: consider whether we might want the capability to
	// have different permissions for different meta endpoints.
	if err := r.Context.AuthorizeEntity(rurl, req); err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	r.Monitor.SetKind("meta")
	key, path := handlerKey(req.URL.Path)
	if key == "" {
		// GET id/meta
		// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmeta
		return r.metaNames(), nil
	}
	if key == "any" {
		return r.serveMetaGetAny(rurl, req)
	}
	if handler := r.handlers.Meta[key]; handler != nil {
		results, err := handler.HandleGet([]BulkIncludeHandler{handler}, rurl, []string{path}, req.Form, req)
		if err != nil {
			// Note: preserve error cause from handlers.
			return nil, errgo.Mask(err, errgo.Any)
		}
		result := results[0]
		if isNull(result) {
			return nil, params.ErrMetadataNotFound
		}
		return results[0], nil
	}
	return nil, errgo.WithCausef(nil, params.ErrNotFound, "unknown metadata %q", strings.TrimPrefix(req.URL.Path, "/"))
}

// GET id/meta/any?[include=meta[&include=meta...]]
// https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmetaany
func (r *Router) serveMetaGetAny(id *ResolvedURL, req *http.Request) (interface{}, error) {
	includes := req.Form["include"]
	if len(includes) == 0 {
		return params.MetaAnyResponse{Id: id.PreferredURL()}, nil
	}
	meta, err := r.GetMetadata(id, includes, req)
	if err != nil {
		// Note: preserve error cause from handlers.
		return nil, errgo.Mask(err, errgo.Any)
	}
	return params.MetaAnyResponse{
		Id:   id.PreferredURL(),
		Meta: meta,
	}, nil
}

const jsonContentType = "application/json"

func unmarshalJSONBody(req *http.Request, val interface{}) error {
	if ct := req.Header.Get("Content-Type"); ct != jsonContentType {
		return errgo.WithCausef(nil, params.ErrBadRequest, "unexpected Content-Type %q; expected %q", ct, jsonContentType)
	}
	dec := json.NewDecoder(req.Body)
	if err := dec.Decode(val); err != nil {
		return errgo.Notef(err, "cannot unmarshal body")
	}
	return nil
}

// serveMetaPut serves a PUT request to the metadata for the given id.
// The metadata to be put is in the request body.
// PUT /$id/meta/...
func (r *Router) serveMetaPut(id *ResolvedURL, req *http.Request) error {
	if err := r.Context.AuthorizeEntity(id, req); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	r.Monitor.SetKind("meta")
	var body json.RawMessage
	if err := unmarshalJSONBody(req, &body); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	return r.serveMetaPutBody(id, req, &body)
}

// serveMetaPutBody serves a PUT request to the metadata for the given id.
// The metadata to be put is in body.
// This method is used both for individual metadata PUTs and
// also bulk metadata PUTs.
func (r *Router) serveMetaPutBody(id *ResolvedURL, req *http.Request, body *json.RawMessage) error {
	key, path := handlerKey(req.URL.Path)
	if key == "" {
		return params.ErrForbidden
	}
	if key == "any" {
		// PUT id/meta/any
		var bodyMeta struct {
			Meta map[string]*json.RawMessage
		}
		if err := json.Unmarshal(*body, &bodyMeta); err != nil {
			return errgo.Notef(err, "cannot unmarshal body")
		}
		if err := r.PutMetadata(id, bodyMeta.Meta, req); err != nil {
			return errgo.Mask(err, errgo.Any)
		}
		return nil
	}
	if handler := r.handlers.Meta[key]; handler != nil {
		errs := handler.HandlePut(
			[]BulkIncludeHandler{handler},
			id,
			[]string{path},
			[]*json.RawMessage{body},
			req,
		)
		if len(errs) > 0 && errs[0] != nil {
			// Note: preserve error cause from handlers.
			return errgo.Mask(errs[0], errgo.Any)
		}
		return nil
	}
	return errgo.WithCausef(nil, params.ErrNotFound, "")
}

// isNull reports whether the given value will encode to
// a null JSON value.
func isNull(val interface{}) bool {
	if val == nil {
		return true
	}
	v := reflect.ValueOf(val)
	if kind := v.Kind(); kind != reflect.Map && kind != reflect.Ptr && kind != reflect.Slice {
		return false
	}
	return v.IsNil()
}

// metaNames returns a slice of all the metadata endpoint names.
func (r *Router) metaNames() []string {
	names := make([]string, 0, len(r.handlers.Meta))
	for name := range r.handlers.Meta {
		// Ensure that we don't generate duplicate entries
		// when there's an entry for both "x" and "x/".
		trimmed := strings.TrimSuffix(name, "/")
		if trimmed != name && r.handlers.Meta[trimmed] != nil {
			continue
		}
		names = append(names, trimmed)
	}
	sort.Strings(names)
	return names
}

// serveBulkMeta serves bulk metadata requests (requests to /meta/...).
func (r *Router) serveBulkMeta(w http.ResponseWriter, req *http.Request) error {
	r.Monitor.SetKind("meta")
	switch req.Method {
	case "GET", "HEAD":
		// A bare meta returns all endpoints.
		// See https://github.com/juju/charmstore/blob/v4/docs/API.md#bulk-requests-and-missing-metadata
		if req.URL.Path == "/" || req.URL.Path == "" {
			httprequest.WriteJSON(w, http.StatusOK, r.metaNames())
			return nil
		}
		resp, err := r.serveBulkMetaGet(req)
		if err != nil {
			return errgo.Mask(err, errgo.Any)
		}
		httprequest.WriteJSON(w, http.StatusOK, resp)
		return nil
	case "PUT":
		return r.serveBulkMetaPut(req)
	default:
		return params.ErrMethodNotAllowed
	}
}

// serveBulkMetaGet serves the "bulk" metadata retrieval endpoint
// that can return information on several ids at once.
//
// GET meta/$endpoint?id=$id0[&id=$id1...][$otherflags]
// See https://github.com/juju/charmstore/blob/v4/docs/API.md#get-metaendpoint
func (r *Router) serveBulkMetaGet(req *http.Request) (interface{}, error) {
	ids := req.Form["id"]
	if len(ids) == 0 {
		return nil, errgo.WithCausef(nil, params.ErrBadRequest, "no ids specified in meta request")
	}
	delete(req.Form, "id")
	ignoreAuth, err := ParseBool(req.Form.Get("ignore-auth"))
	if err != nil {
		return nil, errgo.WithCausef(err, params.ErrBadRequest, "")
	}
	delete(req.Form, "ignore-auth")
	r.willIncludeMetadata(req)
	urls := make([]*charm.URL, len(ids))
	for i, id := range ids {
		url, err := parseURL(id)
		if err != nil {
			return nil, errgo.WithCausef(err, params.ErrBadRequest, "")
		}
		urls[i] = url
	}

	rurls, err := r.Context.ResolveURLs(urls)
	if err != nil {
		// Note: preserve error cause from resolveURL.
		return nil, errgo.Mask(err, errgo.Any)
	}
	result := make(map[string]interface{})
	for i, rurl := range rurls {
		if rurl == nil {
			// URLs not found will be omitted from the result.
			// https://github.com/juju/charmstore/blob/v4/docs/API.md#bulk-requests-and-missing-metadata
			continue
		}
		meta, err := r.serveMetaGet(rurl, req)
		if cause := errgo.Cause(err); cause == params.ErrNotFound || cause == params.ErrMetadataNotFound || (ignoreAuth && isAuthorizationError(cause)) {
			// The relevant data does not exist, or it is not public and client
			// asked not to authorize.
			// https://github.com/juju/charmstore/blob/v4/docs/API.md#bulk-requests-and-missing-metadata
			continue
		}
		if err != nil {
			return nil, errgo.Mask(err)
		}
		result[ids[i]] = meta
	}
	return result, nil
}

// ParseBool returns the boolean value represented by the string.
// It accepts "1" or "0". Any other value returns an error.
func ParseBool(value string) (bool, error) {
	switch value {
	case "0", "":
		return false, nil
	case "1":
		return true, nil
	}
	return false, errgo.Newf(`unexpected bool value %q (must be "0" or "1")`, value)
}

// isAuthorizationError reports whether the given error cause is an
// authorization error.
func isAuthorizationError(cause error) bool {
	if cause == params.ErrUnauthorized {
		return true
	}
	_, ok := cause.(*httpbakery.Error)
	return ok
}

// serveBulkMetaPut serves a bulk PUT request to several ids.
// PUT /meta/$endpoint
// See https://github.com/juju/charmstore/blob/v4/docs/API.md#put-metaendpoint
func (r *Router) serveBulkMetaPut(req *http.Request) error {
	if len(req.Form["id"]) > 0 {
		return fmt.Errorf("ids may not be specified in meta PUT request")
	}
	var ids map[string]*json.RawMessage
	if err := unmarshalJSONBody(req, &ids); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrBadRequest))
	}
	var multiErr multiError
	for id, val := range ids {
		if err := r.serveBulkMetaPutOne(req, id, val); err != nil {
			if multiErr == nil {
				multiErr = make(multiError)
			}
			multiErr[id] = errgo.Mask(err, errgo.Any)
		}
	}
	if len(multiErr) != 0 {
		return multiErr
	}
	return nil
}

// serveBulkMetaPutOne serves a PUT to a single id as part of a bulk PUT
// request. It's in a separate function to make the error handling easier.
func (r *Router) serveBulkMetaPutOne(req *http.Request, id string, val *json.RawMessage) error {
	url, err := parseURL(id)
	if err != nil {
		return errgo.Mask(err)
	}
	rurl, err := r.Context.ResolveURL(url)
	if err != nil {
		// Note: preserve error cause from resolveURL.
		return errgo.Mask(err, errgo.Any)
	}
	if err := r.Context.AuthorizeEntity(rurl, req); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	if err := r.serveMetaPutBody(rurl, req, val); err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	return nil
}

// MetaHandler returns the meta handler for the given meta
// path by looking it up in the Meta map.
func (r *Router) MetaHandler(metaPath string) BulkIncludeHandler {
	key, _ := handlerKey(metaPath)
	return r.handlers.Meta[key]
}

// maxMetadataConcurrency specifies the maximum number
// of goroutines started to service a given GetMetadata request.
// 5 is enough to more that cover the number of metadata
// group handlers in the current API.
const maxMetadataConcurrency = 5

// GetMetadata retrieves metadata for the given charm or bundle id,
// including information as specified by the includes slice.
func (r *Router) GetMetadata(id *ResolvedURL, includes []string, req *http.Request) (map[string]interface{}, error) {
	groups := make(map[interface{}][]BulkIncludeHandler)
	includesByGroup := make(map[interface{}][]string)
	for _, include := range includes {
		handler := r.MetaHandler(include)
		if handler == nil {
			return nil, errgo.Newf("unrecognized metadata name %q", include)
		}

		// Get the key that lets us group this handler into the
		// correct bulk group.
		key := handler.Key()
		groups[key] = append(groups[key], handler)
		includesByGroup[key] = append(includesByGroup[key], include)
	}
	results := make(map[string]interface{})
	// TODO when the number of groups is 1 (a common case,
	// using parallel.NewRun is actually slowing things down
	// by creating a goroutine). We could optimise it so that
	// it doesn't actually create a goroutine in that case.
	run := parallel.NewRun(maxMetadataConcurrency)
	var mu sync.Mutex
	for _, g := range groups {
		g := g
		run.Do(func() error {
			// We know that we must have at least one element in the
			// slice here. We could use any member of the slice to
			// actually handle the request, so arbitrarily choose
			// g[0]. Note that g[0].Key() is equal to g[i].Key() for
			// every i in the slice.
			groupIncludes := includesByGroup[g[0].Key()]

			// Paths contains all the path elements after
			// the handler key has been stripped off.
			// TODO(rog) BUG shouldn't this be len(groupIncludes) ?
			paths := make([]string, len(g))
			for i, include := range groupIncludes {
				_, paths[i] = handlerKey(include)
			}
			groupResults, err := g[0].HandleGet(g, id, paths, nil, req)
			if err != nil {
				// TODO(rog) if it's a BulkError, attach
				// the original include path to error (the BulkError
				// should contain the index of the failed one).
				return errgo.Mask(err, errgo.Any)
			}
			mu.Lock()
			for i, result := range groupResults {
				// Omit nil results from map. Note: omit statically typed
				// nil results too to make it easy for handlers to return
				// possibly nil data with a static type.
				// https://github.com/juju/charmstore/blob/v4/docs/API.md#bulk-requests-and-missing-metadata
				if !isNull(result) {
					results[groupIncludes[i]] = result
				}
			}
			mu.Unlock()
			return nil
		})
	}
	if err := run.Wait(); err != nil {
		// We could have got multiple errors, but we'll only return one of them.
		return nil, errgo.Mask(err.(parallel.Errors)[0], errgo.Any)
	}
	return results, nil
}

// PutMetadata puts metadata for the given id. Each key in data holds
// the name of a metadata endpoint; its associated value
// holds the value to be written.
func (r *Router) PutMetadata(id *ResolvedURL, data map[string]*json.RawMessage, req *http.Request) error {
	groups := make(map[interface{}][]BulkIncludeHandler)
	valuesByGroup := make(map[interface{}][]*json.RawMessage)
	pathsByGroup := make(map[interface{}][]string)
	for path, body := range data {
		handler := r.MetaHandler(path)
		if handler == nil {
			return errgo.Newf("unrecognized metadata name %q", path)
		}

		// Get the key that lets us group this handler into the
		// correct bulk group.
		key := handler.Key()
		groups[key] = append(groups[key], handler)
		valuesByGroup[key] = append(valuesByGroup[key], body)

		// Paths contains all the path elements after
		// the handler key has been stripped off.
		pathsByGroup[key] = append(pathsByGroup[key], path)
	}
	var multiErr multiError
	for _, g := range groups {
		// We know that we must have at least one element in the
		// slice here. We could use any member of the slice to
		// actually handle the request, so arbitrarily choose
		// g[0]. Note that g[0].Key() is equal to g[i].Key() for
		// every i in the slice.
		key := g[0].Key()

		paths := pathsByGroup[key]
		// The paths passed to the handler contain all the path elements
		// after the handler key has been stripped off.
		strippedPaths := make([]string, len(paths))
		for i, path := range paths {
			_, strippedPaths[i] = handlerKey(path)
		}

		errs := g[0].HandlePut(g, id, strippedPaths, valuesByGroup[key], req)
		if len(errs) > 0 {
			if multiErr == nil {
				multiErr = make(multiError)
			}
			if len(errs) != len(paths) {
				return fmt.Errorf("unexpected error count; expected %d, got %q", len(paths), errs)
			}
			for i, err := range errs {
				if err != nil {
					multiErr[paths[i]] = err
				}
			}
		}
	}
	if len(multiErr) != 0 {
		return multiErr
	}
	return nil
}

// splitPath returns the first path element
// after path[i:] and the start of the next
// element.
//
// For example, splitPath("/foo/bar/bzr", 4) returns ("bar", 8).
func splitPath(path string, i int) (elem string, nextIndex int) {
	if i < len(path) && path[i] == '/' {
		i++
	}
	j := strings.Index(path[i:], "/")
	if j == -1 {
		return path[i:], len(path)
	}
	j += i
	return path[i:j], j
}

// splitId splits the given URL path into a charm or bundle
// URL and the rest of the path.
func splitId(path string) (url *charm.URL, rest string, err error) {
	path = strings.TrimPrefix(path, "/")
	part, i := splitPath(path, 0)

	// Skip ~<username>.
	if strings.HasPrefix(part, "~") {
		part, i = splitPath(path, i)
	}

	// Skip series.
	if _, ok := series.Series[part]; ok {
		part, i = splitPath(path, i)
	}

	// part should now contain the charm name,
	// and path[0:i] should contain the entire
	// charm id.
	urlStr := strings.TrimSuffix(path[0:i], "/")
	url, err = parseURL(urlStr)
	if err != nil {
		return nil, "", errgo.Mask(err)
	}
	return url, path[i:], nil
}

func mustParseURL(s string) *charm.URL {
	u, err := parseURL(s)
	if err != nil {
		panic(err)
	}
	return u
}

func parseURL(s string) (*charm.URL, error) {
	u, err := charm.ParseURL(s)
	if err != nil {
		return nil, err
	}
	return u, nil
}
