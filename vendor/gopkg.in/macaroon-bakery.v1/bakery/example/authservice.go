package main

import (
	"net/http"

	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

// authService implements an authorization service,
// that can discharge third-party caveats added
// to other macaroons.
func authService(endpoint string, key *bakery.KeyPair) (http.Handler, error) {
	svc, err := bakery.NewService(bakery.NewServiceParams{
		Location: endpoint,
		Key:      key,
		Locator:  bakery.NewPublicKeyRing(),
	})
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	httpbakery.AddDischargeHandler(mux, "/", svc, thirdPartyChecker)
	return mux, nil
}

// thirdPartyChecker is used to check third party caveats added by other
// services. The HTTP request is that of the client - it is attempting
// to gather a discharge macaroon.
//
// Note how this function can return additional first- and third-party
// caveats which will be added to the original macaroon's caveats.
func thirdPartyChecker(req *http.Request, cavId, condition string) ([]checkers.Caveat, error) {
	if condition != "access-allowed" {
		return nil, checkers.ErrCaveatNotRecognized
	}
	// TODO check that the HTTP request has cookies that prove
	// something about the client.
	return []checkers.Caveat{
		httpbakery.SameClientIPAddrCaveat(req),
	}, nil
}
