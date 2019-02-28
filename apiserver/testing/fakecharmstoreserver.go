// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/lxc/lxd/shared/logger"
	"gopkg.in/juju/charm.v6"
	"net/http"
	"net/http/httptest"
	//"net/url"
	//"strings"
)

//func parseCharmstoreUrl(url *url.URL) (version, series, entity, rest string) {
//	parsedUrl := charm.MustParseURLParseURL(url)
//
//	// http://127.0.0.1:36991/v5/precise/wordpress-999999/archive
//	url_ := url.String()
//	if strings.HasPrefix(url_, "http") {
//		parts := strings.SplitN(url_, "/", 7)
//		return parts[3], parts[4], parts[5], parts[6]
//	}
//
//	// /v5/precise/wordpress-999999/archive
//	parts := strings.SplitN(url_, "/", 5)
//	return parts[1], parts[2], parts[3], parts[4]
//}

// HTTPCloseHandler represents a HTTP handler that
// must be closed after use.
//
//( Copied from charmstore.v5)
type HTTPCloseHandler interface {
	Close()
	http.Handler
}

type CharmStoreServer struct {
	*httptest.Server
}

func (c CharmStoreServer) Close() {
	c.Server.Close()
}


func (c CharmStoreServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//_, _, entity, _ := parseCharmstoreUrl(r.URL)

	url := charm.MustParseURL(r.URL.String())
	logger.Infof("%v %v", r.Method, r.URL)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Entity-Id", url.User)
	w.Write([]byte(`{"ok":true}`))
}

func NewServer() (HTTPCloseHandler, error) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Infof("%v %v (via closure)", r.Method, r.URL)
	}))
	return CharmStoreServer{server}, nil
}


//// https://github.com/juju/charmstore/blob/8ca83bf0934778e98dc1fad8e73ea519ffd740e6/internal/v5/api.go
////
//// RouterHandlers returns router handlers that will route requests to
//// the given ReqHandler. This is provided so that different API versions
//// can override selected parts of the handlers to serve their own API
//// while still using ReqHandler to serve the majority of the API.
//func RouterHandlers(h *ReqHandler) *router.Handlers {
//	resolveId := h.ResolvedIdHandler
//	authId := h.AuthIdHandler
//	return &router.Handlers{
//		Global: map[string]http.Handler{
//			"changes/published":    router.HandleJSON(h.serveChangesPublished),
//			"debug":                http.HandlerFunc(h.serveDebug),
//			"debug/pprof/":         newPprofHandler(h),
//			"debug/status":         router.HandleJSON(h.serveDebugStatus),
//			"list":                 router.HandleJSON(h.serveList),
//			"log":                  router.HandleErrors(h.serveLog),
//			"logout":               http.HandlerFunc(logout),
//			"search":               router.HandleJSON(h.serveSearch),
//			"search/interesting":   http.HandlerFunc(h.serveSearchInteresting),
//			"set-auth-cookie":      router.HandleErrors(h.serveSetAuthCookie),
//			"stats/":               router.NotFoundHandler(),
//			"stats/counter/":       router.HandleJSON(h.serveStatsCounter),
//			"stats/update":         router.HandleErrors(h.serveStatsUpdate),
//			"macaroon":             router.HandleJSON(h.serveMacaroon),
//			"delegatable-macaroon": router.HandleJSON(h.serveDelegatableMacaroon),
//			"whoami":               router.HandleJSON(h.serveWhoAmI),
//			"upload":               router.HandleErrors(h.serveUploadId),
//			"upload/":              router.HandleErrors(h.serveUploadPart),
//		},
//		Id: map[string]router.IdHandler{
//			"archive":                     h.serveArchive,
//			"archive/":                    resolveId(authId(h.serveArchiveFile), "blobhash", "blobhash"),
//			"diagram.svg":                 resolveId(authId(h.serveDiagram), "bundledata"),
//			"expand-id":                   resolveId(authId(h.serveExpandId)),
//			"icon.svg":                    resolveId(authId(h.serveIcon), "contents", "blobhash"),
//			"publish":                     resolveId(h.servePublish),
//			"promulgate":                  resolveId(h.servePromulgate),
//			"readme":                      resolveId(authId(h.serveReadMe), "contents", "blobhash"),
//			"resource/":                   reqBodyReadHandler(resolveId(authId(h.serveResources), "charmmeta")),
//			"docker-resource-upload-info": resolveId(h.serveDockerResourceUploadInfo, "charmmeta"),
//			"allperms":                    h.serveAllPerms,
//		},
//		Meta: map[string]router.BulkIncludeHandler{
//			"archive-size":         h.EntityHandler(h.metaArchiveSize, "size"),
//			"archive-upload-time":  h.EntityHandler(h.metaArchiveUploadTime, "uploadtime"),
//			"bundle-machine-count": h.EntityHandler(h.metaBundleMachineCount, "bundlemachinecount"),
//			"bundle-metadata":      h.EntityHandler(h.metaBundleMetadata, "bundledata"),
//			"bundles-containing":   h.EntityHandler(h.metaBundlesContaining),
//			"bundle-unit-count":    h.EntityHandler(h.metaBundleUnitCount, "bundleunitcount"),
//			"can-ingest":           h.baseEntityHandler(h.metaCanIngest, "noingest"),
//			"can-write":            h.baseEntityHandler(h.metaCanWrite),
//			"charm-actions":        h.EntityHandler(h.metaCharmActions, "charmactions"),
//			"charm-config":         h.EntityHandler(h.metaCharmConfig, "charmconfig"),
//			"charm-metadata":       h.EntityHandler(h.metaCharmMetadata, "charmmeta"),
//			"charm-metrics":        h.EntityHandler(h.metaCharmMetrics, "charmmetrics"),
//			"charm-related":        h.EntityHandler(h.metaCharmRelated, "charmprovidedinterfaces", "charmrequiredinterfaces"),
//			"common-info": h.puttableBaseEntityHandler(
//				h.metaCommonInfo,
//				h.putMetaCommonInfo,
//				"commoninfo",
//			),
//			"common-info/": h.puttableBaseEntityHandler(
//				h.metaCommonInfoWithKey,
//				h.putMetaCommonInfoWithKey,
//				"commoninfo",
//			),
//			"extra-info": h.puttableEntityHandler(
//				h.metaExtraInfo,
//				h.putMetaExtraInfo,
//				"extrainfo",
//			),
//			"extra-info/": h.puttableEntityHandler(
//				h.metaExtraInfoWithKey,
//				h.putMetaExtraInfoWithKey,
//				"extrainfo",
//			),
//			"hash256":          h.EntityHandler(h.metaHash256, "blobhash256"),
//			"hash":             h.EntityHandler(h.metaHash, "blobhash"),
//			"id":               h.EntityHandler(h.metaId, "_id"),
//			"id-name":          h.EntityHandler(h.metaIdName, "_id"),
//			"id-revision":      h.EntityHandler(h.metaIdRevision, "_id"),
//			"id-series":        h.EntityHandler(h.metaIdSeries, "_id"),
//			"id-user":          h.EntityHandler(h.metaIdUser, "_id"),
//			"manifest":         h.EntityHandler(h.metaManifest, "blobhash"),
//			"owner":            h.EntityHandler(h.metaOwner, "_id"),
//			"perm":             h.puttableBaseEntityHandler(h.metaPerm, h.putMetaPerm, "channelacls"),
//			"perm/":            h.puttableBaseEntityHandler(h.metaPermWithKey, h.putMetaPermWithKey, "channelacls"),
//			"promulgated":      h.baseEntityHandler(h.metaPromulgated, "promulgated"),
//			"promulgated-id":   h.EntityHandler(h.metaPromulgatedId, "_id", "promulgated-url"),
//			"published":        h.EntityHandler(h.metaPublished, "published"),
//			"resources":        h.EntityHandler(h.metaResources, "charmmeta"),
//			"resources/":       h.EntityHandler(h.metaResourcesSingle, "charmmeta"),
//			"revision-info":    router.SingleIncludeHandler(h.metaRevisionInfo),
//			"stats":            h.EntityHandler(h.metaStats, "supportedseries"),
//			"supported-series": h.EntityHandler(h.metaSupportedSeries, "supportedseries"),
//			"tags":             h.EntityHandler(h.metaTags, "charmmeta", "bundledata"),
//			"terms":            h.EntityHandler(h.metaTerms, "charmmeta"),
//			"unpromulgated-id": h.EntityHandler(h.metaUnpromulgatedId, "_id"),
//
//			// endpoints not yet implemented:
//			// "color": router.SingleIncludeHandler(h.metaColor),
//		},
//	}