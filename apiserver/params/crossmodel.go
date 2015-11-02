// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// CrossModelOffer holds information about service's offer.
type CrossModelOffer struct {
	Service   string   `json:"service"`
	Endpoints []string `json:"endpoints"`
	URL       string   `json:"url"`
	Users     []string `json:"users"`
}
