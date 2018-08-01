// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore // import "gopkg.in/juju/charmstore.v5"

import (
	"crypto"
	"crypto/x509"
	"fmt"
	"net/http"
	"sort"
	"time"

	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/mgostorage"
	"gopkg.in/mgo.v2"
	"gopkg.in/natefinch/lumberjack.v2"

	"gopkg.in/juju/charmstore.v5/elasticsearch"
	"gopkg.in/juju/charmstore.v5/internal/blobstore"
	"gopkg.in/juju/charmstore.v5/internal/charmstore"
	"gopkg.in/juju/charmstore.v5/internal/dockerauth"
	"gopkg.in/juju/charmstore.v5/internal/legacy"
	"gopkg.in/juju/charmstore.v5/internal/v4"
	"gopkg.in/juju/charmstore.v5/internal/v5"
)

// Versions of the API that can be served.
const (
	DockerAuth = "docker-registry"
	Legacy     = ""
	V4         = "v4"
	V5         = "v5"
)

var versions = map[string]charmstore.NewAPIHandlerFunc{
	DockerAuth: dockerauth.NewAPIHandler,
	Legacy:     legacy.NewAPIHandler,
	V4:         v4.NewAPIHandler,
	V5:         v5.NewAPIHandler,
}

// HTTPCloseHandler represents a HTTP handler that
// must be closed after use.
type HTTPCloseHandler interface {
	Close()
	http.Handler
}

// Versions returns all known API version strings in alphabetical order.
func Versions() []string {
	vs := make([]string, 0, len(versions))
	for v := range versions {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	return vs
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

// NewServer returns a new handler that handles charm store requests and stores
// its data in the given database. The handler will serve the specified
// versions of the API using the given configuration.
func NewServer(db *mgo.Database, es *elasticsearch.Database, idx string, config ServerParams, serveVersions ...string) (HTTPCloseHandler, error) {
	newAPIs := make(map[string]charmstore.NewAPIHandlerFunc)
	for _, vers := range serveVersions {
		newAPI := versions[vers]
		if newAPI == nil {
			return nil, fmt.Errorf("unknown version %q", vers)
		}
		newAPIs[vers] = newAPI
	}
	var si *charmstore.SearchIndex
	if es != nil {
		si = &charmstore.SearchIndex{
			Database: es,
			Index:    idx,
		}
	}
	return charmstore.NewServer(db, si, charmstore.ServerParams(config), newAPIs)
}
