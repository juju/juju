// Copyright 2014-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This is the internal version of the charmstore package.
// It exposes details to the various API packages
// that we do not wish to expose to the world at large.
package charmstore // import "gopkg.in/juju/charmstore.v5/internal/charmstore"

import (
	"crypto"
	"crypto/x509"
	"net/http"
	"strings"
	"time"

	"github.com/juju/idmclient"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/mgostorage"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/mgo.v2"
	"gopkg.in/natefinch/lumberjack.v2"

	"gopkg.in/juju/charmstore.v5/internal/blobstore"
	"gopkg.in/juju/charmstore.v5/internal/monitoring"
	"gopkg.in/juju/charmstore.v5/internal/router"
)

// An APIHandlerParams contains the parameters provided when calling a
// NewAPIHandlerFunc.
type APIHandlerParams struct {
	ServerParams

	// Pool contains the Pool from which Stores should be collected.
	Pool *Pool

	// IDMClient contains an IDMClient for use by the API handler.
	IDMClient *idmclient.Client

	// Path contains the absolute path within the server for the
	// handler.
	Path string
}

// NewAPIHandlerFunc is a function that returns a new API handler that uses
// the given Store. The absPath parameter holds the root path of the
// API handler.
type NewAPIHandlerFunc func(APIHandlerParams) (HTTPCloseHandler, error)

// HTTPCloseHandler represents a HTTP handler that
// must be closed after use.
type HTTPCloseHandler interface {
	Close()
	http.Handler
}

// ServerParams holds configuration for a new internal API server.
type ServerParams struct {
	// AuthUsername and AuthPassword hold the credentials
	// used for HTTP basic authentication.
	AuthUsername string
	AuthPassword string

	// IdentityLocation holds the location of the third party authorization
	// service to use when creating third party caveats,
	// for example: http://api.jujucharms.com/identity
	IdentityLocation string

	// TermsLocations holds the location of the
	// terms service, which knows about user agreements to
	// Terms and Conditions required by the charm.
	TermsLocation string

	// PublicKeyLocator holds a public key store.
	// It may be nil.
	PublicKeyLocator bakery.PublicKeyLocator

	// AgentUsername and AgentKey hold the credentials used for agent
	// authentication.
	AgentUsername string
	AgentKey      *bakery.KeyPair

	// StatsCacheMaxAge is the maximum length of time between
	// refreshes of entities in the stats cache.
	StatsCacheMaxAge time.Duration

	// SearchCacheMaxAge is the maximum length of time between
	// refreshes of entities in the search cache.
	SearchCacheMaxAge time.Duration

	// MaxMgoSessions specifies a soft limit on the maximum
	// number of mongo sessions used. Each concurrent
	// HTTP request will use one session.
	MaxMgoSessions int

	// HTTPRequestWaitDuration holds the amount of time
	// that an HTTP request will wait for a free connection
	// when the MaxConcurrentHTTPRequests limit is reached.
	HTTPRequestWaitDuration time.Duration

	// AuditLogger optionally holds the logger which will be used to
	// write audit log entries.
	AuditLogger *lumberjack.Logger

	// RootKeyPolicy holds the default policy used when creating
	// macaroon root keys.
	RootKeyPolicy mgostorage.Policy

	// MinUploadPartSize holds the minimum size of
	// an upload part. If it's zero, a default value will be used.
	MinUploadPartSize int64

	// MaxUploadPartSize holds the maximum size of
	// an upload part. If it's zero, a default value will be used.
	MaxUploadPartSize int64

	// MaxUploadParts holds the maximum number of upload
	// parts that can be uploaded in a single upload.
	// If it's zero, a default value will be used.
	MaxUploadParts int

	// RunBlobStoreGC holds whether the server will run
	// the blobstore garbage collector worker.
	RunBlobStoreGC bool

	// NewBlobBackend returns a new blobstore backend
	// that may use the given MongoDB database.
	// If this is nil, a MongoDB backend will be used.
	NewBlobBackend func(db *mgo.Database) blobstore.Backend

	// DockerRegistryAddress contains the address of the docker
	// registry associated with the charmstore.
	DockerRegistryAddress string

	// DockerRegistryAuthCertificates contains the chain of
	// certificates used to validate the DockerRegistryAuthKey.
	DockerRegistryAuthCertificates []*x509.Certificate

	// DockerRegistryAuthKey contains the key to use to sign
	// docker registry authorization tokens.
	DockerRegistryAuthKey crypto.Signer

	// DockerRegistryTokenDuration is the time a docker registry
	// token will be valid for after it is created.
	DockerRegistryTokenDuration time.Duration
}

const defaultRootKeyExpiryDuration = 24 * time.Hour

// NewServer returns a handler that serves the given charm store API
// versions using db to store that charm store data.
// An optional elasticsearch configuration can be specified in si. If
// elasticsearch is not being used then si can be set to nil.
// The key of the versions map is the version name.
// The handler configuration is provided to all version handlers.
//
// The returned Server should be closed after use.
func NewServer(db *mgo.Database, si *SearchIndex, config ServerParams, versions map[string]NewAPIHandlerFunc) (*Server, error) {
	if len(versions) == 0 {
		return nil, errgo.Newf("charm store server must serve at least one version of the API")
	}
	config.IdentityLocation = strings.TrimSuffix(config.IdentityLocation, "/")
	config.TermsLocation = strings.TrimSuffix(config.TermsLocation, "/")
	logger.Infof("identity discharge location: %s", config.IdentityLocation)
	logger.Infof("terms discharge location: %s", config.TermsLocation)
	bparams := bakery.NewServiceParams{
		// TODO The location is attached to any macaroons that we
		// mint. Currently we don't know the location of the current
		// service. We potentially provide a way to configure this,
		// but it probably doesn't matter, as nothing currently uses
		// the macaroon location for anything.
		Location: "charmstore",
		Locator:  config.PublicKeyLocator,
	}
	if config.RootKeyPolicy.ExpiryDuration == 0 {
		config.RootKeyPolicy.ExpiryDuration = defaultRootKeyExpiryDuration
	}
	pool, err := NewPool(db, si, &bparams, config)
	if err != nil {
		return nil, errgo.Notef(err, "cannot make store")
	}
	store := pool.Store()
	defer store.Close()
	if err := migrate(store.DB); err != nil {
		pool.Close()
		return nil, errgo.Notef(err, "database migration failed")
	}
	store.Go(func(store *Store) {
		if err := store.syncSearch(); err != nil {
			logger.Errorf("Cannot populate elasticsearch: %v", err)
		}
	})
	srv := &Server{
		pool: pool,
		mux:  router.NewServeMux(),
	}
	params := APIHandlerParams{
		ServerParams: config,
		Pool:         pool,
	}
	if config.IdentityLocation != "" {
		bclient := httpbakery.NewClient()
		bclient.Key = config.AgentKey
		client, err := idmclient.New(idmclient.NewParams{
			Client:        bclient,
			BaseURL:       config.IdentityLocation,
			AgentUsername: config.AgentUsername,
		})
		if err != nil {
			return nil, errgo.Notef(err, "cannot initialize identity client")
		}
		params.IDMClient = client
	}
	// Version independent API.
	handle(srv.mux, "/debug", newServiceDebugHandler(pool, config, srv.mux))
	handle(srv.mux, "/metrics", prometheusHandler())
	for vers, newAPI := range versions {
		params.Path = "/" + vers
		h, err := newAPI(params)
		if err != nil {
			return nil, errgo.Notef(err, "cannot initialize handler for version %v", vers)
		}
		handle(srv.mux, params.Path, h)
		srv.handlers = append(srv.handlers, h)
	}
	if config.RunBlobStoreGC {
		srv.blobstoreGC = newBlobstoreGC(pool)
	}
	return srv, nil
}

func prometheusHandler() http.Handler {
	h := prometheus.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Use prometheus to monitor its own requests...
		monReq := monitoring.NewRequest(req.Method, "prometheus")
		defer monReq.Done()
		monReq.SetKind("metrics")
		h.ServeHTTP(w, req)
	})
}

type Server struct {
	pool        *Pool
	mux         *router.ServeMux
	handlers    []HTTPCloseHandler
	blobstoreGC *blobstoreGC
}

// ServeHTTP implements http.Handler.ServeHTTP.
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.mux.ServeHTTP(w, req)
}

// Close closes the server. It must be called when the server
// is finished with.
func (s *Server) Close() {
	if s.blobstoreGC != nil {
		if err := worker.Stop(s.blobstoreGC); err != nil {
			logger.Errorf("failed to stop blobstore GC: %v", err)
		}
	}
	s.pool.Close()
	for _, h := range s.handlers {
		h.Close()
	}
	s.handlers = nil
}

// Pool returns the Pool used by the server.
func (s *Server) Pool() *Pool {
	return s.pool
}

func handle(mux *router.ServeMux, path string, handler http.Handler) {
	if path != "/" {
		handler = http.StripPrefix(path, handler)
		path += "/"
	}
	mux.Handle(path, handler)
}
