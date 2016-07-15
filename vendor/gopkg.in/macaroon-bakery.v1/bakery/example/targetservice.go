package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"gopkg.in/errgo.v1"

	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

type targetServiceHandler struct {
	svc          *bakery.Service
	authEndpoint string
	endpoint     string
	mux          *http.ServeMux
}

// targetService implements a "target service", representing
// an arbitrary web service that wants to delegate authorization
// to third parties.
//
func targetService(endpoint, authEndpoint string, authPK *bakery.PublicKey) (http.Handler, error) {
	key, err := bakery.GenerateKey()
	if err != nil {
		return nil, err
	}
	pkLocator := bakery.NewPublicKeyRing()
	svc, err := bakery.NewService(bakery.NewServiceParams{
		Key:      key,
		Location: endpoint,
		Locator:  pkLocator,
	})
	if err != nil {
		return nil, err
	}
	log.Printf("adding public key for location %s: %v", authEndpoint, authPK)
	pkLocator.AddPublicKeyForLocation(authEndpoint, true, authPK)
	mux := http.NewServeMux()
	srv := &targetServiceHandler{
		svc:          svc,
		authEndpoint: authEndpoint,
	}
	mux.HandleFunc("/gold/", srv.serveGold)
	mux.HandleFunc("/silver/", srv.serveSilver)
	return mux, nil
}

func (srv *targetServiceHandler) serveGold(w http.ResponseWriter, req *http.Request) {
	checker := srv.checkers(req, "gold")
	if _, err := httpbakery.CheckRequest(srv.svc, req, nil, checker); err != nil {
		srv.writeError(w, req, "gold", err)
		return
	}
	fmt.Fprintf(w, "all is golden")
}

func (srv *targetServiceHandler) serveSilver(w http.ResponseWriter, req *http.Request) {
	checker := srv.checkers(req, "silver")
	if _, err := httpbakery.CheckRequest(srv.svc, req, nil, checker); err != nil {
		srv.writeError(w, req, "silver", err)
		return
	}
	fmt.Fprintf(w, "every cloud has a silver lining")
}

// checkers implements the caveat checking for the service.
func (svc *targetServiceHandler) checkers(req *http.Request, operation string) checkers.Checker {
	return checkers.CheckerFunc{
		Condition_: "operation",
		Check_: func(_, op string) error {
			if op != operation {
				return fmt.Errorf("macaroon not valid for operation")
			}
			return nil
		},
	}
}

// writeError writes an error to w in response to req. If the error was
// generated because of a required macaroon that the client does not
// have, we mint a macaroon that, when discharged, will grant the client
// the right to execute the given operation.
//
// The logic in this function is crucial to the security of the service
// - it must determine for a given operation what caveats to attach.
func (srv *targetServiceHandler) writeError(w http.ResponseWriter, req *http.Request, operation string, verr error) {
	log.Printf("writing error with operation %q", operation)
	fail := func(code int, msg string, args ...interface{}) {
		if code == http.StatusInternalServerError {
			msg = "internal error: " + msg
		}
		http.Error(w, fmt.Sprintf(msg, args...), code)
	}

	if _, ok := errgo.Cause(verr).(*bakery.VerificationError); !ok {
		fail(http.StatusForbidden, "%v", verr)
		return
	}

	// Work out what caveats we need to apply for the given operation.
	// Could special-case the operation here if desired.
	caveats := []checkers.Caveat{
		checkers.TimeBeforeCaveat(time.Now().Add(5 * time.Minute)),
		{
			Location:  srv.authEndpoint,
			Condition: "access-allowed",
		}, {
			Condition: "operation " + operation,
		}}
	// Mint an appropriate macaroon and send it back to the client.
	m, err := srv.svc.NewMacaroon("", nil, caveats)
	if err != nil {
		fail(http.StatusInternalServerError, "cannot mint macaroon: %v", err)
		return
	}
	httpbakery.WriteDischargeRequiredErrorForRequest(w, m, "", verr, req)
}
