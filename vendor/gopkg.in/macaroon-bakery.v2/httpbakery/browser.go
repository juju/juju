package httpbakery

import (
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/juju/webbrowser"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"

	"gopkg.in/macaroon-bakery.v2/bakery"
)

const WebBrowserInteractionKind = "browser-window"

// WaitTokenResponse holds the response type
// returned, JSON-encoded, from the waitToken
// URL passed to SetBrowserInteraction.
type WaitTokenResponse struct {
	Kind string `json:"kind"`
	// Token holds the token value when it's well-formed utf-8
	Token string `json:"token,omitempty"`
	// Token64 holds the token value, base64 encoded, when it's
	// not well-formed utf-8.
	Token64 string `json:"token64,omitempty"`
}

// WaitResponse holds the type that should be returned
// by an HTTP response made to a LegacyWaitURL
// (See the ErrorInfo type).
type WaitResponse struct {
	Macaroon *bakery.Macaroon
}

// WebBrowserInteractionInfo holds the information
// expected in the browser-window interaction
// entry in an interaction-required error.
type WebBrowserInteractionInfo struct {
	// VisitURL holds the URL to be visited in a web browser.
	VisitURL string

	// WaitTokenURL holds a URL that will block on GET
	// until the browser interaction has completed.
	// On success, the response is expected to hold a waitTokenResponse
	// in its body holding the token to be returned from the
	// Interact method.
	WaitTokenURL string
}

var (
	_ Interactor       = WebBrowserInteractor{}
	_ LegacyInteractor = WebBrowserInteractor{}
)

// OpenWebBrowser opens a web browser at the
// given URL. If the OS is not recognised, the URL
// is just printed to standard output.
func OpenWebBrowser(url *url.URL) error {
	err := webbrowser.Open(url)
	if err == nil {
		fmt.Fprintf(os.Stderr, "Opening an authorization web page in your browser.\n")
		fmt.Fprintf(os.Stderr, "If it does not open, please open this URL:\n%s\n", url)
		return nil
	}
	if err == webbrowser.ErrNoBrowser {
		fmt.Fprintf(os.Stderr, "Please open this URL in your browser to authorize:\n%s\n", url)
		return nil
	}
	return err
}

// SetWebBrowserInteraction adds information about web-browser-based
// interaction to the given error, which should be an
// interaction-required error that's about to be returned from a
// discharge request.
//
// The visitURL parameter holds a URL that should be visited by the user
// in a web browser; the waitTokenURL parameter holds a URL that can be
// long-polled to acquire the resulting discharge token.
//
// Use SetLegacyInteraction to add support for legacy clients
// that don't understand the newer InteractionMethods field.
func SetWebBrowserInteraction(e *Error, visitURL, waitTokenURL string) {
	e.SetInteraction(WebBrowserInteractionKind, WebBrowserInteractionInfo{
		VisitURL:     visitURL,
		WaitTokenURL: waitTokenURL,
	})
}

// SetLegacyInteraction adds information about web-browser-based
// interaction (or other kinds of legacy-protocol interaction) to the
// given error, which should be an interaction-required error that's
// about to be returned from a discharge request.
//
// The visitURL parameter holds a URL that should be visited by the user
// in a web browser (or with an "Accept: application/json" header to
// find out the set of legacy interaction methods).
//
// The waitURL parameter holds a URL that can be long-polled
// to acquire the discharge macaroon.
func SetLegacyInteraction(e *Error, visitURL, waitURL string) {
	if e.Info == nil {
		e.Info = new(ErrorInfo)
	}
	e.Info.LegacyVisitURL = visitURL
	e.Info.LegacyWaitURL = waitURL
}

// WebBrowserInteractor handls web-browser-based
// interaction-required errors by opening a web
// browser to allow the user to prove their
// credentials interactively.
//
// It implements the Interactor interface, so instances
// can be used with Client.AddInteractor.
type WebBrowserInteractor struct {
	// OpenWebBrowser is used to visit a page in
	// the user's web browser. If it's nil, the
	// OpenWebBrowser function will be used.
	OpenWebBrowser func(*url.URL) error
}

// Kind implements Interactor.Kind.
func (WebBrowserInteractor) Kind() string {
	return WebBrowserInteractionKind
}

// Interact implements Interactor.Interact by opening a new web page.
func (wi WebBrowserInteractor) Interact(ctx context.Context, client *Client, location string, irErr *Error) (*DischargeToken, error) {
	var p WebBrowserInteractionInfo
	if err := irErr.InteractionMethod(wi.Kind(), &p); err != nil {
		return nil, errgo.Mask(err, errgo.Is(ErrInteractionMethodNotFound))
	}
	visitURL, err := relativeURL(location, p.VisitURL)
	if err != nil {
		return nil, errgo.Notef(err, "cannot make relative visit URL")
	}
	waitTokenURL, err := relativeURL(location, p.WaitTokenURL)
	if err != nil {
		return nil, errgo.Notef(err, "cannot make relative wait URL")
	}
	open := wi.OpenWebBrowser
	if open == nil {
		open = OpenWebBrowser
	}
	if err := open(visitURL); err != nil {
		return nil, errgo.Mask(err)
	}
	return waitForToken(ctx, client, waitTokenURL)
}

// waitForToken returns a token from a the waitToken URL
func waitForToken(ctx context.Context, client *Client, waitTokenURL *url.URL) (*DischargeToken, error) {
	// TODO integrate this with waitForMacaroon somehow?
	httpResp, err := ctxhttp.Get(ctx, client.Client, waitTokenURL.String())
	if err != nil {
		return nil, errgo.Notef(err, "cannot get %q", waitTokenURL)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		err := unmarshalError(httpResp)
		return nil, errgo.NoteMask(err, "cannot acquire discharge token", errgo.Any)
	}
	var resp WaitTokenResponse
	if err := httprequest.UnmarshalJSONResponse(httpResp, &resp); err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal wait response")
	}
	tokenVal, err := maybeBase64Decode(resp.Token, resp.Token64)
	if err != nil {
		return nil, errgo.Notef(err, "bad discharge token")
	}
	// TODO check that kind and value are non-empty?
	return &DischargeToken{
		Kind:  resp.Kind,
		Value: tokenVal,
	}, nil
}

// LegacyInteract implements LegacyInteractor by opening a web browser page.
func (wi WebBrowserInteractor) LegacyInteract(ctx context.Context, client *Client, location string, visitURL *url.URL) error {
	return OpenWebBrowser(visitURL)
}
