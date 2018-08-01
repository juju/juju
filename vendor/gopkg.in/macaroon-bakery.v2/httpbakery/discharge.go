package httpbakery

import (
	"encoding/base64"
	"net/http"
	"path"
	"unicode/utf8"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon.v2"

	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/checkers"
)

// ThirdPartyCaveatChecker is used to check third party caveats.
type ThirdPartyCaveatChecker interface {
	// CheckThirdPartyCaveat is used to check whether a client
	// making the given request should be allowed a discharge for
	// the given caveat. On success, the caveat will be discharged,
	// with any returned caveats also added to the discharge
	// macaroon.
	//
	// The given token, if non-nil, is a token obtained from
	// Interactor.Interact as the result of a discharge interaction
	// after an interaction required error.
	//
	// Note than when used in the context of a discharge handler
	// created by Discharger, any returned errors will be marshaled
	// as documented in DischargeHandler.ErrorMapper.
	CheckThirdPartyCaveat(ctx context.Context, info *bakery.ThirdPartyCaveatInfo, req *http.Request, token *DischargeToken) ([]checkers.Caveat, error)
}

// ThirdPartyCaveatCheckerFunc implements ThirdPartyCaveatChecker
// by calling a function.
type ThirdPartyCaveatCheckerFunc func(ctx context.Context, req *http.Request, info *bakery.ThirdPartyCaveatInfo, token *DischargeToken) ([]checkers.Caveat, error)

func (f ThirdPartyCaveatCheckerFunc) CheckThirdPartyCaveat(ctx context.Context, info *bakery.ThirdPartyCaveatInfo, req *http.Request, token *DischargeToken) ([]checkers.Caveat, error) {
	return f(ctx, req, info, token)
}

// newDischargeClient returns a discharge client that addresses the
// third party discharger at the given location URL and uses
// the given client to make HTTP requests.
//
// If client is nil, http.DefaultClient is used.
func newDischargeClient(location string, client httprequest.Doer) *dischargeClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &dischargeClient{
		Client: httprequest.Client{
			BaseURL:        location,
			Doer:           client,
			UnmarshalError: unmarshalError,
		},
	}
}

// Discharger holds parameters for creating a new Discharger.
type DischargerParams struct {
	// Checker is used to actually check the caveats.
	Checker ThirdPartyCaveatChecker

	// Key holds the key pair of the discharger.
	Key *bakery.KeyPair

	// Locator is used to find public keys when adding
	// third-party caveats on discharge macaroons.
	// If this is nil, no third party caveats may be added.
	Locator bakery.ThirdPartyLocator

	// ErrorToResponse is used to convert errors returned by the third
	// party caveat checker to the form that will be JSON-marshaled
	// on the wire. If zero, this defaults to ErrorToResponse.
	// If set, it should handle errors that it does not understand
	// by falling back to calling ErrorToResponse to ensure
	// that the standard bakery errors are marshaled in the expected way.
	ErrorToResponse func(ctx context.Context, err error) (int, interface{})
}

// Discharger represents a third-party caveat discharger.
// can discharge caveats in an HTTP server.
//
// The name space served by dischargers is as follows.
// All parameters can be provided either as URL attributes
// or form attributes. The result is always formatted as a JSON
// object.
//
// On failure, all endpoints return an error described by
// the Error type.
//
// POST /discharge
//	params:
//		id: all-UTF-8 third party caveat id
//		id64: non-padded URL-base64 encoded caveat id
//		macaroon-id: (optional) id to give to discharge macaroon (defaults to id)
//		token: (optional) value of discharge token
//		token64: (optional) base64-encoded value of discharge token.
//		token-kind: (mandatory if token or token64 provided) discharge token kind.
//	result on success (http.StatusOK):
//		{
//			Macaroon *macaroon.Macaroon
//		}
//
// GET /publickey
//	result:
//		public key of service
//		expiry time of key
type Discharger struct {
	p DischargerParams
}

// NewDischarger returns a new third-party caveat discharger
// using the given parameters.
func NewDischarger(p DischargerParams) *Discharger {
	if p.ErrorToResponse == nil {
		p.ErrorToResponse = ErrorToResponse
	}
	if p.Locator == nil {
		p.Locator = emptyLocator{}
	}
	return &Discharger{
		p: p,
	}
}

type emptyLocator struct{}

func (emptyLocator) ThirdPartyInfo(ctx context.Context, loc string) (bakery.ThirdPartyInfo, error) {
	return bakery.ThirdPartyInfo{}, bakery.ErrNotFound
}

// AddMuxHandlers adds handlers to the given ServeMux to provide
// a third-party caveat discharge service.
func (d *Discharger) AddMuxHandlers(mux *http.ServeMux, rootPath string) {
	for _, h := range d.Handlers() {
		// Note: this only works because we don't have any wildcard
		// patterns in the discharger paths.
		mux.Handle(path.Join(rootPath, h.Path), mkHTTPHandler(h.Handle))
	}
}

// Handlers returns a slice of handlers that can handle a third-party
// caveat discharge service when added to an httprouter.Router.
// TODO provide some way of customizing the context so that
// ErrorToResponse can see a request-specific context.
func (d *Discharger) Handlers() []httprequest.Handler {
	f := func(p httprequest.Params) (dischargeHandler, context.Context, error) {
		return dischargeHandler{
			discharger: d,
		}, p.Context, nil
	}
	srv := httprequest.Server{
		ErrorMapper: d.p.ErrorToResponse,
	}
	return srv.Handlers(f)
}

//go:generate httprequest-generate-client gopkg.in/macaroon-bakery.v2-unstable/httpbakery dischargeHandler dischargeClient

// dischargeHandler is the type used to define the httprequest handler
// methods for a discharger.
type dischargeHandler struct {
	discharger *Discharger
}

// dischargeRequest is a request to create a macaroon that discharges the
// supplied third-party caveat. Discharging caveats will normally be
// handled by the bakery it would be unusual to use this type directly in
// client software.
type dischargeRequest struct {
	httprequest.Route `httprequest:"POST /discharge"`
	Id                string `httprequest:"id,form,omitempty"`
	Id64              string `httprequest:"id64,form,omitempty"`
	Caveat            string `httprequest:"caveat64,form,omitempty"`
	Token             string `httprequest:"token,form,omitempty"`
	Token64           string `httprequest:"token64,form,omitempty"`
	TokenKind         string `httprequest:"token-kind,form,omitempty"`
}

// dischargeResponse contains the response from a /discharge POST request.
type dischargeResponse struct {
	Macaroon *bakery.Macaroon `json:",omitempty"`
}

// Discharge discharges a third party caveat.
func (h dischargeHandler) Discharge(p httprequest.Params, r *dischargeRequest) (*dischargeResponse, error) {
	id, err := maybeBase64Decode(r.Id, r.Id64)
	if err != nil {
		return nil, errgo.Notef(err, "bad caveat id")
	}
	var caveat []byte
	if r.Caveat != "" {
		// Note that it's important that when r.Caveat is empty,
		// we leave DischargeParams.Caveat as nil (Base64Decode
		// always returns a non-nil byte slice).
		caveat1, err := macaroon.Base64Decode([]byte(r.Caveat))
		if err != nil {
			return nil, errgo.Notef(err, "bad base64-encoded caveat: %v", err)
		}
		caveat = caveat1
	}
	tokenVal, err := maybeBase64Decode(r.Token, r.Token64)
	if err != nil {
		return nil, errgo.Notef(err, "bad discharge token")
	}
	var token *DischargeToken
	if len(tokenVal) != 0 {
		if r.TokenKind == "" {
			return nil, errgo.Notef(err, "discharge token provided without token kind")
		}
		token = &DischargeToken{
			Kind:  r.TokenKind,
			Value: tokenVal,
		}
	}
	m, err := bakery.Discharge(p.Context, bakery.DischargeParams{
		Id:     id,
		Caveat: caveat,
		Key:    h.discharger.p.Key,
		Checker: bakery.ThirdPartyCaveatCheckerFunc(
			func(ctx context.Context, cav *bakery.ThirdPartyCaveatInfo) ([]checkers.Caveat, error) {
				return h.discharger.p.Checker.CheckThirdPartyCaveat(ctx, cav, p.Request, token)
			},
		),
		Locator: h.discharger.p.Locator,
	})
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot discharge", errgo.Any)
	}
	return &dischargeResponse{m}, nil
}

// publicKeyRequest specifies the /publickey endpoint.
type publicKeyRequest struct {
	httprequest.Route `httprequest:"GET /publickey"`
}

// publicKeyResponse is the response to a /publickey GET request.
type publicKeyResponse struct {
	PublicKey *bakery.PublicKey
}

// dischargeInfoRequest specifies the /discharge/info endpoint.
type dischargeInfoRequest struct {
	httprequest.Route `httprequest:"GET /discharge/info"`
}

// dischargeInfoResponse is the response to a /discharge/info GET
// request.
type dischargeInfoResponse struct {
	PublicKey *bakery.PublicKey
	Version   bakery.Version
}

// PublicKey returns the public key of the discharge service.
func (h dischargeHandler) PublicKey(*publicKeyRequest) (publicKeyResponse, error) {
	return publicKeyResponse{
		PublicKey: &h.discharger.p.Key.Public,
	}, nil
}

// DischargeInfo returns information on the discharger.
func (h dischargeHandler) DischargeInfo(*dischargeInfoRequest) (dischargeInfoResponse, error) {
	return dischargeInfoResponse{
		PublicKey: &h.discharger.p.Key.Public,
		Version:   bakery.LatestVersion,
	}, nil
}

// mkHTTPHandler converts an httprouter handler to an http.Handler,
// assuming that the httprouter handler has no wildcard path
// parameters.
func mkHTTPHandler(h httprouter.Handle) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h(w, req, nil)
	})
}

// maybeBase64Encode encodes b as is if it's
// OK to be passed as a URL form parameter,
// or encoded as base64 otherwise.
func maybeBase64Encode(b []byte) (s, s64 string) {
	if utf8.Valid(b) {
		valid := true
		for _, c := range b {
			if c < 32 || c == 127 {
				valid = false
				break
			}
		}
		if valid {
			return string(b), ""
		}
	}
	return "", base64.RawURLEncoding.EncodeToString(b)
}

// maybeBase64Decode implements the inverse of maybeBase64Encode.
func maybeBase64Decode(s, s64 string) ([]byte, error) {
	if s64 != "" {
		data, err := macaroon.Base64Decode([]byte(s64))
		if err != nil {
			return nil, errgo.Mask(err)
		}
		if len(data) == 0 {
			return nil, nil
		}
		return data, nil
	}
	return []byte(s), nil
}
