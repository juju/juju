// Package bakerytest provides test helper functions for
// the bakery.
package bakerytest

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/juju/httprequest"
	"gopkg.in/errgo.v1"

	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
)

// Discharger is a third-party caveat discharger suitable
// for testing. It listens on a local network port for
// discharge requests. It should be shut down by calling
// Close when done with.
type Discharger struct {
	Service *bakery.Service

	server *httptest.Server
}

var skipVerify struct {
	mu            sync.Mutex
	refCount      int
	oldSkipVerify bool
}

func startSkipVerify() {
	v := &skipVerify
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.refCount++; v.refCount > 1 {
		return
	}
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return
	}
	if transport.TLSClientConfig != nil {
		v.oldSkipVerify = transport.TLSClientConfig.InsecureSkipVerify
		transport.TLSClientConfig.InsecureSkipVerify = true
	} else {
		v.oldSkipVerify = false
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}
}

func stopSkipVerify() {
	v := &skipVerify
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.refCount--; v.refCount > 0 {
		return
	}
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return
	}
	// technically this doesn't return us to the original state,
	// as TLSClientConfig may have been nil before but won't
	// be now, but that should be equivalent.
	transport.TLSClientConfig.InsecureSkipVerify = v.oldSkipVerify
}

// NewDischarger returns a new third party caveat discharger
// which uses the given function to check caveats.
// The cond and arg arguments to the function are as returned
// by checkers.ParseCaveat.
//
// If locator is non-nil, it will be used to find public keys
// for any third party caveats returned by the checker.
//
// Calling this function has the side-effect of setting
// InsecureSkipVerify in http.DefaultTransport.TLSClientConfig
// until all the dischargers are closed.
func NewDischarger(
	locator bakery.PublicKeyLocator,
	checker func(req *http.Request, cond, arg string) ([]checkers.Caveat, error),
) *Discharger {
	mux := http.NewServeMux()
	server := httptest.NewTLSServer(mux)
	svc, err := bakery.NewService(bakery.NewServiceParams{
		Location: server.URL,
		Locator:  locator,
	})
	if err != nil {
		panic(err)
	}
	checker1 := func(req *http.Request, cav *bakery.ThirdPartyCaveatInfo) ([]checkers.Caveat, error) {
		cond, arg, err := checkers.ParseCaveat(cav.Condition)
		if err != nil {
			return nil, err
		}
		return checker(req, cond, arg)
	}
	httpbakery.AddDischargeHandler(mux, "/", svc, checker1)
	startSkipVerify()
	return &Discharger{
		Service: svc,
		server:  server,
	}
}

// Close shuts down the server. It may be called more than
// once on the same discharger.
func (d *Discharger) Close() {
	if d.server == nil {
		return
	}
	d.server.Close()
	stopSkipVerify()
	d.server = nil
}

// Location returns the location of the discharger, suitable
// for setting as the location in a third party caveat.
// This will be the URL of the server.
func (d *Discharger) Location() string {
	return d.Service.Location()
}

// PublicKeyForLocation implements bakery.PublicKeyLocator.
func (d *Discharger) PublicKeyForLocation(loc string) (*bakery.PublicKey, error) {
	if loc == d.Location() {
		return d.Service.PublicKey(), nil
	}
	return nil, bakery.ErrNotFound
}

type dischargeResult struct {
	err  error
	cavs []checkers.Caveat
}

type discharge struct {
	cavId []byte
	c     chan dischargeResult
}

// InteractiveDischarger is a Discharger that always requires interraction to
// complete the discharge.
type InteractiveDischarger struct {
	Discharger
	Mux *http.ServeMux

	// mu protects the following fields.
	mu      sync.Mutex
	waiting map[string]discharge
	id      int
}

// NewInteractiveDischarger returns a new InteractiveDischarger. The
// InteractiveDischarger will serve the following endpoints by default:
//
//     /discharge - always causes interaction to be required.
//     /publickey - gets the bakery public key.
//     /visit - delegates to visitHandler.
//     /wait - blocks waiting for the interaction to complete.
//
// Additional endpoints may be added to Mux as necessary.
//
// The /discharge endpoint generates a error with the code
// httpbakery.ErrInterractionRequired. The visitURL and waitURL will
// point to the /visit and /wait endpoints of the InteractiveDischarger
// respectively. These URLs will also carry context information in query
// parameters, any handlers should be careful to preserve this context
// information between calls. The easiest way to do this is to always use
// the URL method when generating new URLs.
//
// The /visit endpoint is handled by the provided visitHandler. This
// handler performs the required interactions and should result in the
// FinishInteraction method being called. This handler may process the
// interaction in a number of steps, possibly using additional handlers,
// so long as FinishInteraction is called when no further interaction is
// required.
//
// The /wait endpoint blocks until FinishInteraction has been called by
// the corresponding /visit endpoint, or another endpoint triggered by
// visitHandler.
//
// If locator is non-nil, it will be used to find public keys
// for any third party caveats returned by the checker.
//
// Calling this function has the side-effect of setting
// InsecureSkipVerify in http.DefaultTransport.TLSClientConfig
// until all the dischargers are closed.
//
// The returned InteractiveDischarger must be closed when finished with.
func NewInteractiveDischarger(locator bakery.PublicKeyLocator, visitHandler http.Handler) *InteractiveDischarger {
	d := &InteractiveDischarger{
		Mux:     http.NewServeMux(),
		waiting: map[string]discharge{},
	}
	d.Mux.Handle("/visit", visitHandler)
	d.Mux.Handle("/wait", http.HandlerFunc(d.wait))
	server := httptest.NewTLSServer(d.Mux)
	svc, err := bakery.NewService(bakery.NewServiceParams{
		Location: server.URL,
		Locator:  locator,
	})
	if err != nil {
		panic(err)
	}
	httpbakery.AddDischargeHandler(d.Mux, "/", svc, d.checker)
	startSkipVerify()
	d.Discharger = Discharger{
		Service: svc,
		server:  server,
	}
	return d
}

func (d *InteractiveDischarger) checker(req *http.Request, cav *bakery.ThirdPartyCaveatInfo) ([]checkers.Caveat, error) {
	d.mu.Lock()
	id := fmt.Sprintf("%d", d.id)
	d.id++
	d.waiting[id] = discharge{cav.CaveatId, make(chan dischargeResult, 1)}
	d.mu.Unlock()
	visitURL := "/visit?waitid=" + id
	waitURL := "/wait?waitid=" + id
	return nil, httpbakery.NewInteractionRequiredError(visitURL, waitURL, nil, req)
}

func (d *InteractiveDischarger) wait(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	d.mu.Lock()
	discharge, ok := d.waiting[r.Form.Get("waitid")]
	d.mu.Unlock()
	if !ok {
		code, body := httpbakery.ErrorToResponse(errgo.Newf("invalid waitid %q", r.Form.Get("waitid")))
		httprequest.WriteJSON(w, code, body)
		return
	}
	defer func() {
		d.mu.Lock()
		delete(d.waiting, r.Form.Get("waitid"))
		d.mu.Unlock()
	}()
	var err error
	var cavs []checkers.Caveat
	select {
	case res := <-discharge.c:
		err = res.err
		cavs = res.cavs
	case <-time.After(5 * time.Minute):
		code, body := httpbakery.ErrorToResponse(errgo.New("timeout waiting for interaction to complete"))
		httprequest.WriteJSON(w, code, body)
		return
	}
	if err != nil {
		code, body := httpbakery.ErrorToResponse(err)
		httprequest.WriteJSON(w, code, body)
		return
	}
	m, err := d.Service.Discharge(
		bakery.ThirdPartyCheckerFunc(
			func(cav *bakery.ThirdPartyCaveatInfo) ([]checkers.Caveat, error) {
				return cavs, nil
			},
		),
		discharge.cavId,
	)
	if err != nil {
		code, body := httpbakery.ErrorToResponse(err)
		httprequest.WriteJSON(w, code, body)
		return
	}
	httprequest.WriteJSON(
		w,
		http.StatusOK,
		httpbakery.WaitResponse{
			Macaroon: m,
		},
	)
}

// FinishInteraction signals to the InteractiveDischarger that a
// particular interaction is complete. It causes any waiting requests to
// return. If err is not nil then it will be returned by the
// corresponding /wait request.
func (d *InteractiveDischarger) FinishInteraction(w http.ResponseWriter, r *http.Request, cavs []checkers.Caveat, err error) {
	r.ParseForm()
	d.mu.Lock()
	discharge, ok := d.waiting[r.Form.Get("waitid")]
	d.mu.Unlock()
	if !ok {
		code, body := httpbakery.ErrorToResponse(errgo.Newf("invalid waitid %q", r.Form.Get("waitid")))
		httprequest.WriteJSON(w, code, body)
		return
	}
	select {
	case discharge.c <- dischargeResult{err: err, cavs: cavs}:
	default:
		panic("cannot finish interaction " + r.Form.Get("waitid"))
	}
	return
}

// HostRelativeURL is like URL but includes only the
// URL path and query parameters. Use this when returning
// a URL for use in GetInteractionMethods.
func (d *InteractiveDischarger) HostRelativeURL(path string, r *http.Request) string {
	r.ParseForm()
	return path + "?waitid=" + r.Form.Get("waitid")
}

// URL returns a URL addressed to the given path in the discharger that
// contains any discharger context information found in the given
// request. Use this to generate intermediate URLs before calling
// FinishInteraction.
func (d *InteractiveDischarger) URL(path string, r *http.Request) string {
	r.ParseForm()
	return d.Location() + d.HostRelativeURL(path, r)
}
