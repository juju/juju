package httpbakery

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/net/publicsuffix"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon.v2"

	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
)

var unmarshalError = httprequest.ErrorUnmarshaler(&Error{})

// maxDischargeRetries holds the maximum number of times that an HTTP
// request will be retried after a third party caveat has been successfully
// discharged.
const maxDischargeRetries = 3

// DischargeError represents the error when a third party discharge
// is refused by a server.
type DischargeError struct {
	// Reason holds the underlying remote error that caused the
	// discharge to fail.
	Reason *Error
}

func (e *DischargeError) Error() string {
	return fmt.Sprintf("third party refused discharge: %v", e.Reason)
}

// IsDischargeError reports whether err is a *DischargeError.
func IsDischargeError(err error) bool {
	_, ok := err.(*DischargeError)
	return ok
}

// InteractionError wraps an error returned by a call to visitWebPage.
type InteractionError struct {
	// Reason holds the actual error returned from visitWebPage.
	Reason error
}

func (e *InteractionError) Error() string {
	return fmt.Sprintf("cannot start interactive session: %v", e.Reason)
}

// IsInteractionError reports whether err is an *InteractionError.
func IsInteractionError(err error) bool {
	_, ok := err.(*InteractionError)
	return ok
}

// NewHTTPClient returns an http.Client that ensures
// that headers are sent to the server even when the
// server redirects a GET request. The returned client
// also contains an empty in-memory cookie jar.
//
// See https://github.com/golang/go/issues/4677
func NewHTTPClient() *http.Client {
	c := *http.DefaultClient
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if len(via) == 0 {
			return nil
		}
		for attr, val := range via[0].Header {
			if attr == "Cookie" {
				// Cookies are added automatically anyway.
				continue
			}
			if _, ok := req.Header[attr]; !ok {
				req.Header[attr] = val
			}
		}
		return nil
	}
	jar, err := cookiejar.New(&cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	})
	if err != nil {
		panic(err)
	}
	c.Jar = jar
	return &c
}

// Client holds the context for making HTTP requests
// that automatically acquire and discharge macaroons.
type Client struct {
	// Client holds the HTTP client to use. It should have a cookie
	// jar configured, and when redirecting it should preserve the
	// headers (see NewHTTPClient).
	*http.Client

	// InteractionMethods holds a slice of supported interaction
	// methods, with preferred methods earlier in the slice.
	// On receiving an interaction-required error when discharging,
	// the Kind method of each Interactor in turn will be called
	// and, if the error indicates that the interaction kind is supported,
	// the Interact method will be called to complete the discharge.
	InteractionMethods []Interactor

	// Key holds the client's key. If set, the client will try to
	// discharge third party caveats with the special location
	// "local" by using this key. See bakery.DischargeAllWithKey and
	// bakery.LocalThirdPartyCaveat for more information
	Key *bakery.KeyPair

	// Logger is used to log information about client activities.
	// If it is nil, bakery.DefaultLogger("httpbakery") will be used.
	Logger bakery.Logger
}

// An Interactor represents a way of persuading a discharger
// that it should grant a discharge macaroon.
type Interactor interface {
	// Kind returns the interaction method name. This corresponds to the
	// key in the Error.InteractionMethods type.
	Kind() string

	// Interact performs the interaction, and returns a token that can be
	// used to acquire the discharge macaroon. The location provides
	// the third party caveat location to make it possible to use
	// relative URLs.
	//
	// If the given interaction isn't supported by the client for
	// the given location, it may return an error with an
	// ErrInteractionMethodNotFound cause which will cause the
	// interactor to be ignored that time.
	Interact(ctx context.Context, client *Client, location string, interactionRequiredErr *Error) (*DischargeToken, error)
}

// DischargeToken holds a token that is intended
// to persuade a discharger to discharge a third
// party caveat.
type DischargeToken struct {
	// Kind holds the kind of the token. By convention this
	// matches the name of the interaction method used to
	// obtain the token, but that's not required.
	Kind string `json:"kind"`

	// Value holds the value of the token.
	Value []byte `json:"value"`
}

// LegacyInteractor may optionally be implemented by Interactor
// implementations that implement the legacy interaction-required
// error protocols.
type LegacyInteractor interface {
	// LegacyInteract implements the "visit" half of a legacy discharge
	// interaction. The "wait" half will be implemented by httpbakery.
	// The location is the location specified by the third party
	// caveat.
	LegacyInteract(ctx context.Context, client *Client, location string, visitURL *url.URL) error
}

// NewClient returns a new Client containing an HTTP client
// created with NewHTTPClient and leaves all other fields zero.
func NewClient() *Client {
	return &Client{
		Client: NewHTTPClient(),
	}
}

// AddInteractor is a convenience method that appends the given
// interactor to c.InteractionMethods.
// For example, to enable web-browser interaction on
// a client c, do:
//
//	c.AddInteractor(httpbakery.WebBrowserWindowInteractor)
func (c *Client) AddInteractor(i Interactor) {
	c.InteractionMethods = append(c.InteractionMethods, i)
}

// DischargeAll attempts to acquire discharge macaroons for all the
// third party caveats in m, and returns a slice containing all
// of them bound to m.
//
// If the discharge fails because a third party refuses to discharge a
// caveat, the returned error will have a cause of type *DischargeError.
// If the discharge fails because visitWebPage returns an error,
// the returned error will have a cause of *InteractionError.
//
// The returned macaroon slice will not be stored in the client
// cookie jar (see SetCookie if you need to do that).
func (c *Client) DischargeAll(ctx context.Context, m *bakery.Macaroon) (macaroon.Slice, error) {
	return bakery.DischargeAllWithKey(ctx, m, c.AcquireDischarge, c.Key)
}

// DischargeAllUnbound is like DischargeAll except that it does not
// bind the resulting macaroons.
func (c *Client) DischargeAllUnbound(ctx context.Context, ms bakery.Slice) (bakery.Slice, error) {
	return ms.DischargeAll(ctx, c.AcquireDischarge, c.Key)
}

// Do is like DoWithContext, except the context is automatically derived.
// If using go version 1.7 or later the context will be taken from the
// given request, otherwise context.Background() will be used.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.do(contextFromRequest(req), req, nil)
}

// DoWithContext sends the given HTTP request and returns its response.
// If the request fails with a discharge-required error, any required
// discharge macaroons will be acquired, and the request will be repeated
// with those attached.
//
// If the required discharges were refused by a third party, an error
// with a *DischargeError cause will be returned.
//
// If interaction is required by the user, the client's InteractionMethods
// will be used to perform interaction. An error
// with a *InteractionError cause will be returned if this interaction
// fails. See WebBrowserWindowInteractor for a possible implementation of
// an Interactor for an interaction method.
//
// DoWithContext may add headers to req.Header.
func (c *Client) DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error) {
	return c.do(ctx, req, nil)
}

// DoWithCustomError is like Do except it allows a client
// to specify a custom error function, getError, which is called on the
// HTTP response and may return a non-nil error if the response holds an
// error. If the cause of the returned error is a *Error value and its
// code is ErrDischargeRequired, the macaroon in its Info field will be
// discharged and the request will be repeated with the discharged
// macaroon. If getError returns nil, it should leave the response body
// unchanged.
//
// If getError is nil, DefaultGetError will be used.
//
// This method can be useful when dealing with APIs that
// return their errors in a format incompatible with Error, but the
// need for it should be avoided when creating new APIs,
// as it makes the endpoints less amenable to generic tools.
func (c *Client) DoWithCustomError(req *http.Request, getError func(resp *http.Response) error) (*http.Response, error) {
	return c.do(contextFromRequest(req), req, getError)
}

func (c *Client) do(ctx context.Context, req *http.Request, getError func(resp *http.Response) error) (*http.Response, error) {
	c.logDebugf(ctx, "client do %s %s {", req.Method, req.URL)
	resp, err := c.do1(ctx, req, getError)
	c.logDebugf(ctx, "} -> error %#v", err)
	return resp, err
}

func (c *Client) do1(ctx context.Context, req *http.Request, getError func(resp *http.Response) error) (*http.Response, error) {
	if getError == nil {
		getError = DefaultGetError
	}
	if c.Client.Jar == nil {
		return nil, errgo.New("no cookie jar supplied in HTTP client")
	}
	rreq, ok := newRetryableRequest(c.Client, req)
	if !ok {
		return nil, fmt.Errorf("request body is not seekable")
	}
	defer rreq.close()

	req.Header.Set(BakeryProtocolHeader, fmt.Sprint(bakery.LatestVersion))

	// Make several attempts to do the request, because we might have
	// to get through several layers of security. We only retry if
	// we get a DischargeRequiredError and succeed in discharging
	// the macaroon in it.
	retry := 0
	for {
		resp, err := c.do2(ctx, rreq, getError)
		if err == nil || !isDischargeRequiredError(err) {
			return resp, errgo.Mask(err, errgo.Any)
		}
		if retry++; retry > maxDischargeRetries {
			return nil, errgo.NoteMask(err, fmt.Sprintf("too many (%d) discharge requests", retry-1), errgo.Any)
		}
		if err1 := c.HandleError(ctx, req.URL, err); err1 != nil {
			return nil, errgo.Mask(err1, errgo.Any)
		}
		c.logDebugf(ctx, "discharge succeeded; retry %d", retry)
	}
}

func (c *Client) do2(ctx context.Context, rreq *retryableRequest, getError func(resp *http.Response) error) (*http.Response, error) {
	httpResp, err := rreq.do(ctx)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Any)
	}
	err = getError(httpResp)
	if err == nil {
		c.logInfof(ctx, "HTTP response OK (status %v)", httpResp.Status)
		return httpResp, nil
	}
	httpResp.Body.Close()
	return nil, errgo.Mask(err, errgo.Any)
}

// HandleError tries to resolve the given error, which should be a
// response to the given URL, by discharging any macaroon contained in
// it. That is, if the error cause is an *Error and its code is
// ErrDischargeRequired, then it will try to discharge
// err.Info.Macaroon. If the discharge succeeds, the discharged macaroon
// will be saved to the client's cookie jar and ResolveError will return
// nil.
//
// For any other kind of error, the original error will be returned.
func (c *Client) HandleError(ctx context.Context, reqURL *url.URL, err error) error {
	respErr, ok := errgo.Cause(err).(*Error)
	if !ok {
		return err
	}
	if respErr.Code != ErrDischargeRequired {
		return respErr
	}
	if respErr.Info == nil || respErr.Info.Macaroon == nil {
		return errgo.New("no macaroon found in discharge-required response")
	}
	mac := respErr.Info.Macaroon
	macaroons, err := bakery.DischargeAllWithKey(ctx, mac, c.AcquireDischarge, c.Key)
	if err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	var cookiePath string
	if path := respErr.Info.MacaroonPath; path != "" {
		relURL, err := parseURLPath(path)
		if err != nil {
			c.logInfof(ctx, "ignoring invalid path in discharge-required response: %v", err)
		} else {
			cookiePath = reqURL.ResolveReference(relURL).Path
		}
	}
	// TODO use a namespace taken from the error response.
	cookie, err := NewCookie(nil, macaroons)
	if err != nil {
		return errgo.Notef(err, "cannot make cookie")
	}
	cookie.Path = cookiePath
	if name := respErr.Info.CookieNameSuffix; name != "" {
		cookie.Name = "macaroon-" + name
	}
	c.Jar.SetCookies(reqURL, []*http.Cookie{cookie})
	return nil
}

// DefaultGetError is the default error unmarshaler used by Client.Do.
func DefaultGetError(httpResp *http.Response) error {
	if httpResp.StatusCode != http.StatusProxyAuthRequired && httpResp.StatusCode != http.StatusUnauthorized {
		return nil
	}
	// Check for the new protocol discharge error.
	if httpResp.StatusCode == http.StatusUnauthorized && httpResp.Header.Get("WWW-Authenticate") != "Macaroon" {
		return nil
	}
	if httpResp.Header.Get("Content-Type") != "application/json" {
		return nil
	}
	var resp Error
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return fmt.Errorf("cannot unmarshal error response: %v", err)
	}
	return &resp
}

func parseURLPath(path string) (*url.URL, error) {
	u, err := url.Parse(path)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if u.Scheme != "" ||
		u.Opaque != "" ||
		u.User != nil ||
		u.Host != "" ||
		u.RawQuery != "" ||
		u.Fragment != "" {
		return nil, errgo.Newf("URL path %q is not clean", path)
	}
	return u, nil
}

// PermanentExpiryDuration holds the length of time a cookie
// holding a macaroon with no time-before caveat will be
// stored.
const PermanentExpiryDuration = 100 * 365 * 24 * time.Hour

// NewCookie takes a slice of macaroons and returns them
// encoded as a cookie. The slice should contain a single primary
// macaroon in its first element, and any discharges after that.
//
// The given namespace specifies the first party caveat namespace,
// used for deriving the expiry time of the cookie.
func NewCookie(ns *checkers.Namespace, ms macaroon.Slice) (*http.Cookie, error) {
	if len(ms) == 0 {
		return nil, errgo.New("no macaroons in cookie")
	}
	// TODO(rog) marshal cookie as binary if version allows.
	data, err := json.Marshal(ms)
	if err != nil {
		return nil, errgo.Notef(err, "cannot marshal macaroons")
	}
	cookie := &http.Cookie{
		Name:  fmt.Sprintf("macaroon-%x", ms[0].Signature()),
		Value: base64.StdEncoding.EncodeToString(data),
	}
	expires, found := checkers.MacaroonsExpiryTime(ns, ms)
	if !found {
		// The macaroon doesn't expire - use a very long expiry
		// time for the cookie.
		expires = time.Now().Add(PermanentExpiryDuration)
	} else if expires.Sub(time.Now()) < time.Minute {
		// The macaroon might have expired already, or it's
		// got a short duration, so treat it as a session cookie
		// by setting Expires to the zero time.
		expires = time.Time{}
	}
	cookie.Expires = expires
	// TODO(rog) other fields.
	return cookie, nil
}

// SetCookie sets a cookie for the given URL on the given cookie jar
// that will holds the given macaroon slice. The macaroon slice should
// contain a single primary macaroon in its first element, and any
// discharges after that.
//
// The given namespace specifies the first party caveat namespace,
// used for deriving the expiry time of the cookie.
func SetCookie(jar http.CookieJar, url *url.URL, ns *checkers.Namespace, ms macaroon.Slice) error {
	cookie, err := NewCookie(ns, ms)
	if err != nil {
		return errgo.Mask(err)
	}
	jar.SetCookies(url, []*http.Cookie{cookie})
	return nil
}

// MacaroonsForURL returns any macaroons associated with the
// given URL in the given cookie jar.
func MacaroonsForURL(jar http.CookieJar, u *url.URL) []macaroon.Slice {
	return cookiesToMacaroons(jar.Cookies(u))
}

func appendURLElem(u, elem string) string {
	if strings.HasSuffix(u, "/") {
		return u + elem
	}
	return u + "/" + elem
}

// AcquireDischarge acquires a discharge macaroon from the caveat location as an HTTP URL.
// It fits the getDischarge argument type required by bakery.DischargeAll.
func (c *Client) AcquireDischarge(ctx context.Context, cav macaroon.Caveat, payload []byte) (*bakery.Macaroon, error) {
	m, err := c.acquireDischarge(ctx, cav, payload, nil)
	if err == nil {
		return m, nil
	}
	cause, ok := errgo.Cause(err).(*Error)
	if !ok {
		return nil, errgo.NoteMask(err, "cannot acquire discharge", IsInteractionError)
	}
	if cause.Code != ErrInteractionRequired {
		return nil, &DischargeError{
			Reason: cause,
		}
	}
	if cause.Info == nil {
		return nil, errgo.Notef(err, "interaction-required response with no info")
	}
	// Make sure the location has a trailing slash so that
	// the relative URL calculations work correctly even when
	// cav.Location doesn't have a trailing slash.
	loc := appendURLElem(cav.Location, "")
	token, m, err := c.interact(ctx, loc, cause, payload)
	if err != nil {
		return nil, errgo.Mask(err, IsDischargeError, IsInteractionError)
	}
	if m != nil {
		// We've acquired the macaroon directly via legacy interaction.
		return m, nil
	}

	// Try to acquire the discharge again, but this time with
	// the token acquired by the interaction method.
	m, err = c.acquireDischarge(ctx, cav, payload, token)
	if err != nil {
		return nil, errgo.Mask(err, IsDischargeError, IsInteractionError)
	}
	return m, nil
}

// acquireDischarge is like AcquireDischarge except that it also
// takes a token acquired from an interaction method.
func (c *Client) acquireDischarge(
	ctx context.Context,
	cav macaroon.Caveat,
	payload []byte,
	token *DischargeToken,
) (*bakery.Macaroon, error) {
	dclient := newDischargeClient(cav.Location, c)
	var req dischargeRequest
	req.Id, req.Id64 = maybeBase64Encode(cav.Id)
	if token != nil {
		req.Token, req.Token64 = maybeBase64Encode(token.Value)
		req.TokenKind = token.Kind
	}
	req.Caveat = base64.RawURLEncoding.EncodeToString(payload)
	resp, err := dclient.Discharge(ctx, &req)
	if err == nil {
		return resp.Macaroon, nil
	}
	return nil, errgo.Mask(err, errgo.Any)
}

// interact gathers a macaroon by directing the user to interact with a
// web page. The irErr argument holds the interaction-required
// error response.
func (c *Client) interact(ctx context.Context, location string, irErr *Error, payload []byte) (*DischargeToken, *bakery.Macaroon, error) {
	if len(c.InteractionMethods) == 0 {
		return nil, nil, &InteractionError{
			Reason: errgo.New("interaction required but not possible"),
		}
	}
	if irErr.Info.InteractionMethods == nil && irErr.Info.LegacyVisitURL != "" {
		// It's an old-style error; deal with it differently.
		m, err := c.legacyInteract(ctx, location, irErr)
		if err != nil {
			return nil, nil, errgo.Mask(err, IsDischargeError, IsInteractionError)
		}
		return nil, m, nil
	}
	for _, interactor := range c.InteractionMethods {
		c.logDebugf(ctx, "checking interaction method %q", interactor.Kind())
		if _, ok := irErr.Info.InteractionMethods[interactor.Kind()]; ok {
			c.logDebugf(ctx, "found possible interaction method %q", interactor.Kind())
			token, err := interactor.Interact(ctx, c, location, irErr)
			if err != nil {
				if errgo.Cause(err) == ErrInteractionMethodNotFound {
					continue
				}
				return nil, nil, errgo.Mask(err, IsDischargeError, IsInteractionError)
			}
			if token == nil {
				return nil, nil, errgo.New("interaction method returned an empty token")
			}
			return token, nil, nil
		} else {
			c.logDebugf(ctx, "interaction method %q not found in %#v", interactor.Kind(), irErr.Info.InteractionMethods)
		}
	}
	return nil, nil, &InteractionError{
		Reason: errgo.Newf("no supported interaction method"),
	}
}

func (c *Client) legacyInteract(ctx context.Context, location string, irErr *Error) (*bakery.Macaroon, error) {
	visitURL, err := relativeURL(location, irErr.Info.LegacyVisitURL)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	waitURL, err := relativeURL(location, irErr.Info.LegacyWaitURL)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	methodURLs := map[string]*url.URL{
		"interactive": visitURL,
	}
	if len(c.InteractionMethods) > 1 || c.InteractionMethods[0].Kind() != WebBrowserInteractionKind {
		// We have several possible methods or we only support a non-window
		// method, so we need to fetch the possible methods supported by the discharger.
		methodURLs = legacyGetInteractionMethods(ctx, c.logger(), c, visitURL)
	}
	for _, interactor := range c.InteractionMethods {
		kind := interactor.Kind()
		if kind == WebBrowserInteractionKind {
			// This is the old name for browser-window interaction.
			kind = "interactive"
		}
		interactor, ok := interactor.(LegacyInteractor)
		if !ok {
			// Legacy interaction mode isn't supported.
			continue
		}
		visitURL, ok := methodURLs[kind]
		if !ok {
			continue
		}
		visitURL, err := relativeURL(location, visitURL.String())
		if err != nil {
			return nil, errgo.Mask(err)
		}
		if err := interactor.LegacyInteract(ctx, c, location, visitURL); err != nil {
			return nil, &InteractionError{
				Reason: errgo.Mask(err, errgo.Any),
			}
		}
		return waitForMacaroon(ctx, c, waitURL)
	}
	return nil, &InteractionError{
		Reason: errgo.Newf("no methods supported"),
	}
}

func (c *Client) logDebugf(ctx context.Context, f string, a ...interface{}) {
	c.logger().Debugf(ctx, f, a...)
}

func (c *Client) logInfof(ctx context.Context, f string, a ...interface{}) {
	c.logger().Infof(ctx, f, a...)
}

func (c *Client) logger() bakery.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return bakery.DefaultLogger("httpbakery")
}

// waitForMacaroon returns a macaroon from a legacy wait endpoint.
func waitForMacaroon(ctx context.Context, client *Client, waitURL *url.URL) (*bakery.Macaroon, error) {
	httpResp, err := ctxhttp.Get(ctx, client.Client, waitURL.String())
	if err != nil {
		return nil, errgo.Notef(err, "cannot get %q", waitURL)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		err := unmarshalError(httpResp)
		if err1, ok := err.(*Error); ok {
			err = &DischargeError{
				Reason: err1,
			}
		}
		return nil, errgo.NoteMask(err, "failed to acquire macaroon after waiting", errgo.Any)
	}
	var resp WaitResponse
	if err := httprequest.UnmarshalJSONResponse(httpResp, &resp); err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal wait response")
	}
	return resp.Macaroon, nil
}

// relativeURL returns newPath relative to an original URL.
func relativeURL(base, new string) (*url.URL, error) {
	if new == "" {
		return nil, errgo.Newf("empty URL")
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, errgo.Notef(err, "cannot parse URL")
	}
	newURL, err := url.Parse(new)
	if err != nil {
		return nil, errgo.Notef(err, "cannot parse URL")
	}
	return baseURL.ResolveReference(newURL), nil
}

// TODO(rog) move a lot of the code below into server.go, as it's
// much more about server side than client side.

// MacaroonsHeader is the key of the HTTP header that can be used to provide a
// macaroon for request authorization.
const MacaroonsHeader = "Macaroons"

// RequestMacaroons returns any collections of macaroons from the header and
// cookies found in the request. By convention, each slice will contain a
// primary macaroon followed by its discharges.
func RequestMacaroons(req *http.Request) []macaroon.Slice {
	mss := cookiesToMacaroons(req.Cookies())
	for _, h := range req.Header[MacaroonsHeader] {
		ms, err := decodeMacaroonSlice(h)
		if err != nil {
			// Ignore invalid macaroons.
			continue
		}
		mss = append(mss, ms)
	}
	return mss
}

// cookiesToMacaroons returns a slice of any macaroons found
// in the given slice of cookies.
func cookiesToMacaroons(cookies []*http.Cookie) []macaroon.Slice {
	var mss []macaroon.Slice
	for _, cookie := range cookies {
		if !strings.HasPrefix(cookie.Name, "macaroon-") {
			continue
		}
		ms, err := decodeMacaroonSlice(cookie.Value)
		if err != nil {
			// Ignore invalid macaroons.
			continue
		}
		mss = append(mss, ms)
	}
	return mss
}

// decodeMacaroonSlice decodes a base64-JSON-encoded slice of macaroons from
// the given string.
func decodeMacaroonSlice(value string) (macaroon.Slice, error) {
	data, err := macaroon.Base64Decode([]byte(value))
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot base64-decode macaroons")
	}
	// TODO(rog) accept binary encoded macaroon cookies.
	var ms macaroon.Slice
	if err := json.Unmarshal(data, &ms); err != nil {
		return nil, errgo.NoteMask(err, "cannot unmarshal macaroons")
	}
	return ms, nil
}
