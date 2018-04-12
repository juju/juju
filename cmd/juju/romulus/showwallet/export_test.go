// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package showwallet

import (
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

var (
	NewWalletAPIClient = &newWalletAPIClient
	NewJujuclientStore = &newJujuclientStore
)

// WalletAPIClientFnc returns a function that returns the provided walletAPIClient
// and can be used to patch the NewWalletAPIClient variable for tests.
func WalletAPIClientFnc(api walletAPIClient) func(*httpbakery.Client) (walletAPIClient, error) {
	return func(*httpbakery.Client) (walletAPIClient, error) {
		return api, nil
	}
}
