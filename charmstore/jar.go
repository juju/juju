// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"
)

// MacaroonURI is use when register new Juju checkers with the bakery.
const MacaroonURI = "github.com/juju/juju"

// MacaroonNamespace is the namespace Juju uses for managing macaroons.
var MacaroonNamespace = checkers.NewNamespace(map[string]string{MacaroonURI: ""})

// newMacaroonJar returns a new macaroonJar wrapping the given cache and
// expecting to be used against the given URL.  Both the cache and url must
// be non-nil.
func newMacaroonJar(cache MacaroonCache, serverURL *url.URL) (*macaroonJar, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &macaroonJar{
		underlying: jar,
		cache:      cache,
		serverURL:  serverURL,
	}, nil
}

// macaroonJar is a special form of http.CookieJar that uses a backing
// MacaroonCache to populate the jar and store updated macaroons.
// This is a fairly specifically crafted type in order to deal with the fact
// that gopkg.in/juju/charmrepo.v2/csclient.Client does all the work
// of handling updated macaroons.  If a request with a macaroon returns with a
// DischargeRequiredError, csclient.Client will discharge the returned
// macaroon's caveats, and then save the final macaroon in the cookiejar, then
// retry the request.  This type handles populating the macaroon from the
// macaroon cache (which for Juju's purposes will be a wrapper around state),
// and then responds to csclient's setcookies call to save the new macaroon
// into state for the appropriate charm.
//
// Note that Activate and Deactivate are not safe to call concurrently.
type macaroonJar struct {
	underlying   http.CookieJar
	cache        MacaroonCache
	serverURL    *url.URL
	currentCharm *charm.URL
	err          error
}

// Activate empties the cookiejar and loads the macaroon for the given charm
// (if any) into the cookiejar, avoiding the mechanism in SetCookies
// that records new macaroons.  This also enables the functionality of storing
// macaroons in SetCookies.
// If the macaroonJar is nil, this is NOP.
func (j *macaroonJar) Activate(cURL *charm.URL) error {
	if j == nil {
		return nil
	}
	if err := j.reset(); err != nil {
		return errors.Trace(err)
	}
	j.currentCharm = cURL

	m, err := j.cache.Get(cURL)
	if err != nil {
		return errors.Trace(err)
	}
	if m != nil {
		httpbakery.SetCookie(j.underlying, j.serverURL, MacaroonNamespace, m)
	}
	return nil
}

// Deactivate empties the cookiejar and disables the functionality of storing
// macaroons in SetCookies.
// If the macaroonJar is nil, this is NOP.
func (j *macaroonJar) Deactivate() error {
	if j == nil {
		return nil
	}
	return j.reset()
}

// reset empties the cookiejar and disables the functionality of storing
// macaroons in SetCookies.
func (j *macaroonJar) reset() error {
	j.err = nil
	j.currentCharm = nil

	// clear out the cookie jar to ensure we don't have any cruft left over.
	underlying, err := cookiejar.New(nil)
	if err != nil {
		// currently this is impossible, since the above never actually
		// returns an error
		return errors.Trace(err)
	}
	j.underlying = underlying
	return nil
}

// SetCookies handles the receipt of the cookies in a reply for the
// given URL.  Cookies do not persist past a successive call to Activate or
// Deactivate.  If the jar is currently activated, macaroons set via this method
// will be stored in the underlying MacaroonCache for the currently activated
// charm.
func (j *macaroonJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.underlying.SetCookies(u, cookies)

	if j.currentCharm == nil {
		// nothing else to do
		return
	}

	mac, err := extractMacaroon(cookies)
	if err != nil {
		j.err = errors.Trace(err)
		logger.Errorf(err.Error())
		return
	}
	if mac == nil {
		return
	}
	if err := j.cache.Set(j.currentCharm, mac); err != nil {
		j.err = errors.Trace(err)
		logger.Errorf("Failed to store macaroon for %s: %s", j.currentCharm, err)
	}
}

// Cookies returns the cookies stored in the underlying cookiejar.
func (j macaroonJar) Cookies(u *url.URL) []*http.Cookie {
	return j.underlying.Cookies(u)
}

// Error returns any error encountered during SetCookies.
func (j *macaroonJar) Error() error {
	if j == nil {
		return nil
	}
	return j.err
}

func extractMacaroon(cookies []*http.Cookie) (macaroon.Slice, error) {
	macs := httpbakery.MacaroonsForURL(jarFromCookies(cookies), nil)
	switch len(macs) {
	case 0:
		// no macaroons in cookies, that's ok.
		return nil, nil
	case 1:
		// hurray!
		return macs[0], nil
	default:
		return nil, errors.Errorf("Expected exactly one macaroon, received %d", len(macs))
	}
}

// jarFromCookies is a bit of sleight of hand to get the cookies we already
// have to be in the form of a http.CookieJar that is suitable for using with
// the httpbakery's MacaroonsForURL function, which expects to extract
// the cookies from a cookiejar first.
type jarFromCookies []*http.Cookie

func (jarFromCookies) SetCookies(_ *url.URL, _ []*http.Cookie) {}

func (j jarFromCookies) Cookies(_ *url.URL) []*http.Cookie {
	return []*http.Cookie(j)
}
