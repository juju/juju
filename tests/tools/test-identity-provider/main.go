// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakerytest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
)

// test-identity-provider is a minimal bakery-based identity provider for use
// in integration tests. It auto-approves every "is-authenticated-user" third-
// party caveat discharge as the username supplied via --username, eliminating
// the need for a full Candid/JIMM deployment to test @external user flows.
//
// Usage:
//
//	test-identity-provider --username <name>
//
// Prints two lines to stdout and then serves until killed:
//
//	<HTTP URL of the discharger>
//	<base64url-encoded public key>
//
// Bootstrap Juju pointing at this server:
//
//	juju bootstrap ... \
//	    --config identity-url=<URL> \
//	    --config identity-public-key=<KEY> \
//	    --config allow-model-access=true
//
// The identity-public-key config is required when using an HTTP (non-TLS)
// identity-url; providing the key lets Juju skip the HTTPS requirement for
// fetching the key from the identity service.
func main() {
	username := flag.String("username", "", "username to auto-approve for every login request")
	flag.Parse()

	if *username == "" {
		fmt.Fprintln(os.Stderr, "must specify --username")
		os.Exit(1)
	}

	// Use HTTP so that Juju can be pointed at this server with just
	// identity-url + identity-public-key, without TLS certificate trust.
	httpbakery.AllowInsecureThirdPartyLocator = true

	discharger := bakerytest.NewDischarger(nil)
	defer discharger.Close()

	approvedUser := *username
	discharger.CheckerP = httpbakery.ThirdPartyCaveatCheckerPFunc(
		func(ctx context.Context, p httpbakery.ThirdPartyCaveatCheckerParams) ([]checkers.Caveat, error) {
			cond, _, err := checkers.ParseCaveat(string(p.Caveat.Condition))
			if err != nil {
				return nil, err
			}
			if cond != "is-authenticated-user" {
				return nil, fmt.Errorf("unknown caveat condition: %q", cond)
			}
			return []checkers.Caveat{checkers.DeclaredCaveat("username", approvedUser)}, nil
		},
	)

	pubKeyBytes, err := discharger.Key.Public.MarshalText()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to marshal public key:", err)
		os.Exit(1)
	}

	// Print URL then public key, each on its own line, so callers can capture them.
	fmt.Println(discharger.Location())
	fmt.Println(string(pubKeyBytes))

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
}
