package httpbakery

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"gopkg.in/errgo.v1"

	"gopkg.in/macaroon-bakery.v1/bakery"
)

// NewPublicKeyRing returns a new public keyring that uses
// the given client to find public keys and uses the
// given cache as a backing. If cache is nil, a new
// cache will be created. If client is nil, http.DefaultClient will
// be used.
func NewPublicKeyRing(client *http.Client, cache *bakery.PublicKeyRing) *PublicKeyRing {
	if cache == nil {
		cache = bakery.NewPublicKeyRing()
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &PublicKeyRing{
		client: client,
		cache:  cache,
	}
}

// PublicKeyRing represents a public keyring that can interrogate
// remote services for their public keys. By default it refuses
// to use insecure URLs.
type PublicKeyRing struct {
	client        *http.Client
	allowInsecure bool
	cache         *bakery.PublicKeyRing
}

// AllowInsecure allows insecure URLs. This can be useful
// for testing purposes.
func (kr *PublicKeyRing) AllowInsecure() {
	kr.allowInsecure = true
}

// PublicKeyForLocation implements bakery.PublicKeyLocator
// by first looking in the backing cache and, if that fails,
// making an HTTP request to the public key associated
// with the given discharge location.
func (kr *PublicKeyRing) PublicKeyForLocation(loc string) (*bakery.PublicKey, error) {
	u, err := url.Parse(loc)
	if err != nil {
		return nil, errgo.Notef(err, "invalid discharge URL %q", loc)
	}
	if u.Scheme != "https" && !kr.allowInsecure {
		return nil, errgo.Newf("untrusted discharge URL %q", loc)
	}
	k, err := kr.cache.PublicKeyForLocation(loc)
	if err == nil {
		return k, nil
	}
	k, err = PublicKeyForLocation(kr.client, loc)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if err := kr.cache.AddPublicKeyForLocation(loc, true, k); err != nil {
		// Cannot happen in practice as it will only fail if
		// loc is an invalid URL which we have already checked.
		return nil, errgo.Notef(err, "cannot cache discharger URL %q", loc)
	}
	return k, nil
}

// PublicKeyForLocation returns the public key from a macaroon
// discharge server running at the given location URL.
// Note that this is insecure if an http: URL scheme is used.
// If client is nil, http.DefaultClient will be used.
func PublicKeyForLocation(client *http.Client, url string) (*bakery.PublicKey, error) {
	if client == nil {
		client = http.DefaultClient
	}
	url = strings.TrimSuffix(url, "/") + "/publickey"
	resp, err := client.Get(url)
	if err != nil {
		return nil, errgo.Notef(err, "cannot get public key from %q", url)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errgo.Newf("cannot get public key from %q: got status %s", url, resp.Status)
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errgo.Notef(err, "failed to read response body from %q", url)
	}
	var pubkey struct {
		PublicKey *bakery.PublicKey
	}
	err = json.Unmarshal(data, &pubkey)
	if err != nil {
		return nil, errgo.Notef(err, "failed to decode response from %q", url)
	}
	return pubkey.PublicKey, nil
}
