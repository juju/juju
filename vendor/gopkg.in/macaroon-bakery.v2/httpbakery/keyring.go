package httpbakery

import (
	"net/http"
	"net/url"

	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"

	"gopkg.in/macaroon-bakery.v2/bakery"
)

var _ bakery.ThirdPartyLocator = (*ThirdPartyLocator)(nil)

// NewThirdPartyLocator returns a new third party
// locator that uses the given client to find
// information about third parties and
// uses the given cache as a backing.
//
// If cache is nil, a new cache will be created.
//
// If client is nil, http.DefaultClient will be used.
func NewThirdPartyLocator(client httprequest.Doer, cache *bakery.ThirdPartyStore) *ThirdPartyLocator {
	if cache == nil {
		cache = bakery.NewThirdPartyStore()
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &ThirdPartyLocator{
		client: client,
		cache:  cache,
	}
}

// AllowInsecureThirdPartyLocator holds whether ThirdPartyLocator allows
// insecure HTTP connections for fetching third party information.
// It is provided for testing purposes and should not be used
// in production code.
var AllowInsecureThirdPartyLocator = false

// ThirdPartyLocator represents locator that can interrogate
// third party discharge services for information. By default it refuses
// to use insecure URLs.
type ThirdPartyLocator struct {
	client        httprequest.Doer
	allowInsecure bool
	cache         *bakery.ThirdPartyStore
}

// AllowInsecure allows insecure URLs. This can be useful
// for testing purposes. See also AllowInsecureThirdPartyLocator.
func (kr *ThirdPartyLocator) AllowInsecure() {
	kr.allowInsecure = true
}

// ThirdPartyLocator implements bakery.ThirdPartyLocator
// by first looking in the backing cache and, if that fails,
// making an HTTP request to find the information associated
// with the given discharge location.
func (kr *ThirdPartyLocator) ThirdPartyInfo(ctx context.Context, loc string) (bakery.ThirdPartyInfo, error) {
	u, err := url.Parse(loc)
	if err != nil {
		return bakery.ThirdPartyInfo{}, errgo.Notef(err, "invalid discharge URL %q", loc)
	}
	if u.Scheme != "https" && !kr.allowInsecure && !AllowInsecureThirdPartyLocator {
		return bakery.ThirdPartyInfo{}, errgo.Newf("untrusted discharge URL %q", loc)
	}
	info, err := kr.cache.ThirdPartyInfo(ctx, loc)
	if err == nil {
		return info, nil
	}
	info, err = ThirdPartyInfoForLocation(ctx, kr.client, loc)
	if err != nil {
		return bakery.ThirdPartyInfo{}, errgo.Mask(err)
	}
	kr.cache.AddInfo(loc, info)
	return info, nil
}

// ThirdPartyInfoForLocation returns information on the third party
// discharge server running at the given location URL. Note that this is
// insecure if an http: URL scheme is used. If client is nil,
// http.DefaultClient will be used.
func ThirdPartyInfoForLocation(ctx context.Context, client httprequest.Doer, url string) (bakery.ThirdPartyInfo, error) {
	dclient := newDischargeClient(url, client)
	info, err := dclient.DischargeInfo(ctx, &dischargeInfoRequest{})
	if err == nil {
		return bakery.ThirdPartyInfo{
			PublicKey: *info.PublicKey,
			Version:   info.Version,
		}, nil
	}
	derr, ok := errgo.Cause(err).(*httprequest.DecodeResponseError)
	if !ok || derr.Response.StatusCode != http.StatusNotFound {
		return bakery.ThirdPartyInfo{}, errgo.Mask(err)
	}
	// The new endpoint isn't there, so try the old one.
	pkResp, err := dclient.PublicKey(ctx, &publicKeyRequest{})
	if err != nil {
		return bakery.ThirdPartyInfo{}, errgo.Mask(err)
	}
	return bakery.ThirdPartyInfo{
		PublicKey: *pkResp.PublicKey,
		Version:   bakery.Version1,
	}, nil
}
