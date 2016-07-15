// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrepo // import "gopkg.in/juju/charmrepo.v2-unstable"

import (
	"crypto/sha512"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/juju/utils"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
)

// CacheDir stores the charm cache directory path.
var CacheDir string

// CharmStore is a repository Interface that provides access to the public Juju
// charm store.
type CharmStore struct {
	client *csclient.Client
}

var _ Interface = (*CharmStore)(nil)

// NewCharmStoreParams holds parameters for instantiating a new CharmStore.
type NewCharmStoreParams struct {
	// URL holds the root endpoint URL of the charm store,
	// with no trailing slash, not including the version.
	// For example https://api.jujucharms.com/charmstore
	// If empty, the default charm store client location is used.
	URL string

	// BakeryClient holds the bakery client to use when making
	// requests to the store. This is used in preference to
	// HTTPClient.
	BakeryClient *httpbakery.Client

	// HTTPClient holds the HTTP client to use when making
	// requests to the store. If nil, httpbakery.NewHTTPClient will
	// be used.
	HTTPClient *http.Client

	// VisitWebPage is called when authorization requires that
	// the user visits a web page to authenticate themselves.
	// If nil, no interaction will be allowed. This field
	// is ignored if BakeryClient is provided.
	VisitWebPage func(url *url.URL) error

	// User holds the name to authenticate as for the client. If User is empty,
	// no credentials will be sent.
	User string

	// Password holds the password for the given user, for authenticating the
	// client.
	Password string
}

// NewCharmStore creates and returns a charm store repository.
// The given parameters are used to instantiate the charm store.
//
// The errors returned from the interface methods will
// preserve the causes returned from the underlying csclient
// methods.
func NewCharmStore(p NewCharmStoreParams) *CharmStore {
	client := csclient.New(csclient.Params{
		URL:          p.URL,
		BakeryClient: p.BakeryClient,
		HTTPClient:   p.HTTPClient,
		VisitWebPage: p.VisitWebPage,
		User:         p.User,
		Password:     p.Password,
	})
	return NewCharmStoreFromClient(client)
}

// NewCharmStoreFromClient creates and returns a charm store repository.
// The provided client is used for charm store requests.
func NewCharmStoreFromClient(client *csclient.Client) *CharmStore {
	return &CharmStore{
		client: client,
	}
}

// Client returns the charmstore client that the CharmStore
// implementation uses under the hood.
func (s *CharmStore) Client() *csclient.Client {
	return s.client
}

// Get implements Interface.Get.
func (s *CharmStore) Get(curl *charm.URL) (charm.Charm, error) {
	// The cache location must have been previously set.
	if CacheDir == "" {
		panic("charm cache directory path is empty")
	}
	if curl.Series == "bundle" {
		return nil, errgo.Newf("expected a charm URL, got bundle URL %q", curl)
	}
	path, err := s.archivePath(curl)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	return charm.ReadCharmArchive(path)
}

// GetBundle implements Interface.GetBundle.
func (s *CharmStore) GetBundle(curl *charm.URL) (charm.Bundle, error) {
	// The cache location must have been previously set.
	if CacheDir == "" {
		panic("charm cache directory path is empty")
	}
	if curl.Series != "bundle" {
		return nil, errgo.Newf("expected a bundle URL, got charm URL %q", curl)
	}
	path, err := s.archivePath(curl)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	return charm.ReadBundleArchive(path)
}

// archivePath returns a local path to the downloaded archive of the given
// charm or bundle URL, storing it in CacheDir, which it creates if necessary.
// If an archive with a matching SHA hash already exists locally, it will use
// the local version.
func (s *CharmStore) archivePath(curl *charm.URL) (string, error) {
	// Prepare the cache directory and retrieve the entity archive.
	if err := os.MkdirAll(CacheDir, 0755); err != nil {
		return "", errgo.Notef(err, "cannot create the cache directory")
	}
	etype := "charm"
	if curl.Series == "bundle" {
		etype = "bundle"
	}
	r, id, expectHash, expectSize, err := s.client.GetArchive(curl)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			// Make a prettier error message for the user.
			return "", errgo.WithCausef(nil, params.ErrNotFound, "cannot retrieve %q: %s not found", curl, etype)
		}
		return "", errgo.NoteMask(err, fmt.Sprintf("cannot retrieve %s %q", etype, curl), errgo.Any)
	}
	defer r.Close()

	// Check if the archive already exists in the cache.
	path := filepath.Join(CacheDir, charm.Quote(id.String())+"."+etype)
	if verifyHash384AndSize(path, expectHash, expectSize) == nil {
		return path, nil
	}

	// Verify and save the new archive.
	f, err := ioutil.TempFile(CacheDir, "charm-download")
	if err != nil {
		return "", errgo.Notef(err, "cannot make temporary file")
	}
	defer f.Close()
	hash := sha512.New384()
	size, err := io.Copy(io.MultiWriter(hash, f), r)
	if err != nil {
		return "", errgo.Notef(err, "cannot read entity archive")
	}
	if size != expectSize {
		return "", errgo.Newf("size mismatch; network corruption?")
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != expectHash {
		return "", errgo.Newf("hash mismatch; network corruption?")
	}

	// Move the archive to the expected place, and return the charm.

	// Note that we need to close the temporary file before moving
	// it because otherwise Windows prohibits the rename.
	f.Close()
	if err := utils.ReplaceFile(f.Name(), path); err != nil {
		return "", errgo.Notef(err, "cannot move the entity archive")
	}
	return path, nil
}

func verifyHash384AndSize(path, expectHash string, expectSize int64) error {
	f, err := os.Open(path)
	if err != nil {
		return errgo.Mask(err)
	}
	defer f.Close()
	hash := sha512.New384()
	size, err := io.Copy(hash, f)
	if err != nil {
		return errgo.Mask(err)
	}
	if size != expectSize {
		logger.Debugf("size mismatch for %q", path)
		return errgo.Newf("size mismatch for %q", path)
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != expectHash {
		logger.Debugf("hash mismatch for %q", path)
		return errgo.Newf("hash mismatch for %q", path)
	}
	return nil
}

// Resolve implements Interface.Resolve.
func (s *CharmStore) Resolve(ref *charm.URL) (*charm.URL, []string, error) {
	resolved, _, supportedSeries, err := s.ResolveWithChannel(ref)
	if err != nil {
		return nil, nil, errgo.Mask(err, errgo.Any)
	}
	return resolved, supportedSeries, nil
}

// ResolveWithChannel does the same thing as Resolve() but also returns
// the best channel to use.
func (s *CharmStore) ResolveWithChannel(ref *charm.URL) (*charm.URL, params.Channel, []string, error) {
	var result struct {
		Id              params.IdResponse
		SupportedSeries params.SupportedSeriesResponse
		Published       params.PublishedResponse
	}
	if _, err := s.client.Meta(ref, &result); err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			// Make a prettier error message for the user.
			etype := "charm"
			switch ref.Series {
			case "bundle":
				etype = "bundle"
			case "":
				etype = "charm or bundle"
			}
			return nil, params.NoChannel, nil, errgo.WithCausef(nil, params.ErrNotFound, "cannot resolve URL %q: %s not found", ref, etype)
		}
		return nil, params.NoChannel, nil, errgo.NoteMask(err, fmt.Sprintf("cannot resolve charm URL %q", ref), errgo.Any)
	}
	// TODO(ericsnow) Get this directly from the API. It has high risk
	// of getting stale. Perhaps add params.PublishedResponse.BestChannel
	// or, less desireably, have params.PublishedResponse.Info be
	// priority-ordered.
	channel := bestChannel(s.client, result.Published.Info)
	return result.Id.Id, channel, result.SupportedSeries.SupportedSeries, nil
}

// bestChannel determines the best channel to use for the given client
// and published info.
//
// Note that this is equivalent to code on the server side.
// See ReqHandler.entityChannel in internal/v5/auth.go.
func bestChannel(client *csclient.Client, published []params.PublishedInfo) params.Channel {
	explicitChannel := client.Channel()
	if explicitChannel != params.NoChannel {
		return explicitChannel
	}

	bestChannel := params.UnpublishedChannel
	for _, info := range published {
		// TODO(ericsnow) Favor the one with info.Current == true?
		switch info.Channel {
		case params.StableChannel:
			bestChannel = info.Channel
			break
		case params.DevelopmentChannel:
			bestChannel = info.Channel
		default:
			panic(fmt.Sprintf("unknown channel %q", info.Channel))
		}
	}
	return bestChannel
}
