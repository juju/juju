package httpbakery

import (
	"net/http"
	"net/url"

	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
)

// TODO(rog) rename this file.

// legacyGetInteractionMethods queries a URL as found in an
// ErrInteractionRequired VisitURL field to find available interaction
// methods.
//
// It does this by sending a GET request to the URL with the Accept
// header set to "application/json" and parsing the resulting
// response as a map[string]string.
//
// It uses the given Doer to execute the HTTP GET request.
func legacyGetInteractionMethods(ctx context.Context, logger bakery.Logger, client httprequest.Doer, u *url.URL) map[string]*url.URL {
	methodURLs, err := legacyGetInteractionMethods1(ctx, client, u)
	if err != nil {
		// When a discharger doesn't support retrieving interaction methods,
		// we expect to get an error, because it's probably returning an HTML
		// page not JSON.
		if logger != nil {
			logger.Debugf(ctx, "ignoring error: cannot get interaction methods: %v; %s", err, errgo.Details(err))
		}
		methodURLs = make(map[string]*url.URL)
	}
	if methodURLs["interactive"] == nil {
		// There's no "interactive" method returned, but we know
		// the server does actually support it, because all dischargers
		// are required to, so fill it in with the original URL.
		methodURLs["interactive"] = u
	}
	return methodURLs
}

func legacyGetInteractionMethods1(ctx context.Context, client httprequest.Doer, u *url.URL) (map[string]*url.URL, error) {
	httpReqClient := &httprequest.Client{
		Doer: client,
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, errgo.Notef(err, "cannot create request")
	}
	req.Header.Set("Accept", "application/json")
	var methodURLStrs map[string]string
	if err := httpReqClient.Do(ctx, req, &methodURLStrs); err != nil {
		return nil, errgo.Mask(err)
	}
	// Make all the URLs relative to the request URL.
	methodURLs := make(map[string]*url.URL)
	for m, urlStr := range methodURLStrs {
		relURL, err := url.Parse(urlStr)
		if err != nil {
			return nil, errgo.Notef(err, "invalid URL for interaction method %q", m)
		}
		methodURLs[m] = u.ResolveReference(relURL)
	}
	return methodURLs, nil
}
