// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The legacy package implements the legacy API, as follows:
//
// /charm-info
//
// A GET call to `/charm-info` returns info about one or more charms, including
// its canonical URL, revision, SHA256 checksum and VCS revision digest.
// The returned info is in JSON format.
// For instance a request to `/charm-info?charms=cs:trusty/juju-gui` returns the
// following response:
//
//     {"cs:trusty/juju-gui": {
//         "canonical-url": "cs:trusty/juju-gui",
//         "revision": 3,
//         "sha256": "a15c77f3f92a0fb7b61e9...",
//         "digest": jeff.pihach@canonical.com-20140612210347-6cc9su1jqjkhbi84"
//     }}
//
// /charm-event:
//
// A GET call to `/charm-event` returns info about an event occurred in the life
// of the specified charm(s). Currently two types of events are logged:
// "published" (a charm has been published and it's available in the store) and
// "publish-error" (an error occurred while importing the charm).
// E.g. a call to `/charm-event?charms=cs:trusty/juju-gui` generates the following
// JSON response:
//
//     {"cs:trusty/juju-gui": {
//         "kind": "published",
//         "revision": 3,
//         "digest": "jeff.pihach@canonicalcom-20140612210347-6cc9su1jqjkhbi84",
//         "time": "2014-06-16T14:41:19Z"
//     }}
//
// /charm/
//
// The `charm` API provides the ability to download a charm as a Zip archive,
// given the charm identifier. For instance, it is possible to download the Juju
// GUI charm by performing a GET call to `/charm/trusty/juju-gui-42`. Both the
// revision and OS series can be omitted, e.g. `/charm/juju-gui` will download the
// last revision of the Juju GUI charm with support to the more recent Ubuntu LTS
// series.
//
// /stats/counter/
//
// Stats can be retrieved by calling `/stats/counter/{key}` where key is a query
// that specifies the counter stats to calculate and return.
//
// For instance, a call to `/stats/counter/charm-bundle:*` returns the number of
// times a charm has been downloaded from the store. To get the same value for
// a specific charm, it is possible to filter the results by passing the charm
// series and name, e.g. `/stats/counter/charm-bundle:trusty:juju-gui`.
//
// The results can be grouped by specifying the `by` query (possible values are
// `day` and `week`), and time delimited using the `start` and `stop` queries.
//
// It is also possible to list the results by passing `list=1`. For example, a GET
// call to `/stats/counter/charm-bundle:trusty:*?by=day&list=1` returns an
// aggregated count of trusty charms downloads, grouped by charm and day, similar
// to the following:
//
//     charm-bundle:trusty:juju-gui  2014-06-17  5
//     charm-bundle:trusty:mysql     2014-06-17  1
package legacy // import "gopkg.in/juju/charmstore.v5-unstable/internal/legacy"

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/juju/mempool"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"

	"gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
	"gopkg.in/juju/charmstore.v5-unstable/internal/router"
	"gopkg.in/juju/charmstore.v5-unstable/internal/v4"
)

type Handler struct {
	v4 v4.Handler
}

type reqHandler struct {
	v4    v4.ReqHandler
	mux   *http.ServeMux
	store *charmstore.Store
}

// reqHandlerPool holds a cache of ReqHandlers to save
// on allocation time.
var reqHandlerPool = mempool.Pool{
	New: func() interface{} {
		return newReqHandler()
	},
}

func NewAPIHandler(pool *charmstore.Pool, config charmstore.ServerParams, rootPath string) charmstore.HTTPCloseHandler {
	return &Handler{
		v4: v4.New(pool, config, rootPath),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()
	rh, err := h.newReqHandler()
	if err != nil {
		router.WriteError(w, err)
		return
	}
	defer rh.close()
	rh.mux.ServeHTTP(w, req)
}

func (h *Handler) Close() {
}

func (h *Handler) newReqHandler() (*reqHandler, error) {
	v4h, err := h.v4.NewReqHandler(new(http.Request))
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(charmstore.ErrTooManySessions))
	}
	rh := reqHandlerPool.Get().(*reqHandler)
	rh.v4 = v4h
	rh.store = v4h.Store.Store
	return rh, nil
}

// newReqHandler returns a new instance of the legacy API handler.
// The returned value has a nil v4 field.
func newReqHandler() *reqHandler {
	h := &reqHandler{
		mux: http.NewServeMux(),
	}
	h.handle("/charm-info", router.HandleJSON(h.serveCharmInfo))
	h.handle("/charm/", router.HandleErrors(h.serveCharm))
	h.handle("/charm-event", router.HandleJSON(h.serveCharmEvent))
	return h
}

func (h *reqHandler) handle(path string, handler http.Handler) {
	prefix := strings.TrimSuffix(path, "/")
	h.mux.Handle(path, http.StripPrefix(prefix, handler))
}

func (h *reqHandler) close() {
	h.v4.Close()
	h.v4 = v4.ReqHandler{}
	reqHandlerPool.Put(h)
}

func (h *reqHandler) serveCharm(w http.ResponseWriter, req *http.Request) error {
	if req.Method != "GET" && req.Method != "HEAD" {
		return params.ErrMethodNotAllowed
	}
	curl, err := charm.ParseURL(strings.TrimPrefix(req.URL.Path, "/"))
	if err != nil {
		return errgo.WithCausef(err, params.ErrNotFound, "")
	}
	return h.v4.Router.Handlers().Id["archive"](curl, w, req)
}

// charmStatsKey returns a stats key for the given charm reference and kind.
func charmStatsKey(url *charm.URL, kind string) []string {
	if url.User == "" {
		return []string{kind, url.Series, url.Name}
	}
	return []string{kind, url.Series, url.Name, url.User}
}

var errNotFound = fmt.Errorf("entry not found")

func (h *reqHandler) serveCharmInfo(_ http.Header, req *http.Request) (interface{}, error) {
	response := make(map[string]*charmrepo.InfoResponse)
	for _, url := range req.Form["charms"] {
		c := &charmrepo.InfoResponse{}
		response[url] = c
		curl, err := charm.ParseURL(url)
		if err != nil {
			err = errNotFound
		}
		var entity *mongodoc.Entity
		if err == nil {
			entity, err = h.store.FindBestEntity(curl, params.UnpublishedChannel, nil)
			if errgo.Cause(err) == params.ErrNotFound {
				// The old API actually returned "entry not found"
				// on *any* error, but it seems reasonable to be
				// a little more descriptive for other errors.
				err = errNotFound
			}
		}
		var rurl *router.ResolvedURL
		if err == nil {
			rurl = charmstore.EntityResolvedURL(entity)
			if h.v4.AuthorizeEntity(rurl, req) != nil {
				// The charm is unauthorized and there's no way to
				// authorize it as part of the legacy API so we
				// just treat it as a not-found error.
				err = errNotFound
			}
		}
		// Prepare the response part for this charm.
		if err == nil {
			curl = entity.PreferredURL(curl.User == "")
			c.CanonicalURL = curl.String()
			c.Revision = curl.Revision
			c.Sha256 = entity.BlobHash256
			c.Digest, err = entityBzrDigest(entity)
			if err != nil {
				c.Errors = append(c.Errors, err.Error())
			}
			if v4.StatsEnabled(req) {
				h.store.IncCounterAsync(charmStatsKey(curl, params.StatsCharmInfo))
			}
		} else {
			c.Errors = append(c.Errors, err.Error())
			if curl != nil && v4.StatsEnabled(req) {
				h.store.IncCounterAsync(charmStatsKey(curl, params.StatsCharmMissing))
			}
		}
	}
	return response, nil
}

// serveCharmEvent returns events related to the charms specified in the
// "charms" query. In this implementation, the only supported event is
// "published", required by the "juju publish" command.
func (h *reqHandler) serveCharmEvent(_ http.Header, req *http.Request) (interface{}, error) {
	response := make(map[string]*charmrepo.EventResponse)
	for _, url := range req.Form["charms"] {
		c := &charmrepo.EventResponse{}

		// Ignore the digest part of the request.
		if i := strings.Index(url, "@"); i != -1 {
			url = url[:i]
		}
		// We intentionally do not implement the long_keys query parameter that
		// the legacy charm store supported, as "juju publish" does not use it.
		response[url] = c

		// Validate the charm URL.
		id, err := charm.ParseURL(url)
		if err != nil {
			c.Errors = []string{"invalid charm URL: " + err.Error()}
			continue
		}
		if id.Revision != -1 {
			c.Errors = []string{"got charm URL with revision: " + id.String()}
			continue
		}

		// Retrieve the charm.
		entity, err := h.store.FindBestEntity(id, params.UnpublishedChannel, charmstore.FieldSelector("_id", "uploadtime", "extrainfo"))
		if err != nil {
			if errgo.Cause(err) == params.ErrNotFound {
				// The old API actually returned "entry not found"
				// on *any* error, but it seems reasonable to be
				// a little more descriptive for other errors.
				err = errNotFound
			}
			c.Errors = []string{err.Error()}
			continue
		}

		// Retrieve the entity Bazaar digest.
		c.Digest, err = entityBzrDigest(entity)
		if err != nil {
			c.Errors = []string{err.Error()}
		} else if c.Digest == "" {
			// There are two possible reasons why an entity is found without a
			// digest:
			// 1) the entity has been recently added in the ingestion process,
			//    but the extra-info has not been sent yet by "charmload";
			// 2) there was an error while ingesting the entity.
			// If the entity has been recently published, we assume case 1),
			// and therefore we return a not found error, forcing
			// "juju publish" to keep retrying and possibly succeed later.
			// Otherwise, we return an error so that "juju publish" exits with
			// an error and avoids an infinite loop.
			if time.Since(entity.UploadTime).Minutes() < 2 {
				c.Errors = []string{errNotFound.Error()}
			} else {
				c.Errors = []string{"digest not found: this can be due to an error while ingesting the entity"}
			}
			continue
		}

		// Prepare the response part for this charm.
		c.Kind = "published"
		if id.User == "" {
			c.Revision = entity.PromulgatedRevision
		} else {
			c.Revision = entity.Revision
		}
		c.Time = entity.UploadTime.UTC().Format(time.RFC3339)
		if v4.StatsEnabled(req) {
			h.store.IncCounterAsync(charmStatsKey(id, params.StatsCharmEvent))
		}
	}
	return response, nil
}

func entityBzrDigest(entity *mongodoc.Entity) (string, error) {
	value, found := entity.ExtraInfo[params.BzrDigestKey]
	if !found {
		return "", nil
	}
	var digest string
	if err := json.Unmarshal(value, &digest); err != nil {
		return "", errgo.Notef(err, "cannot unmarshal digest")
	}
	return digest, nil
}
