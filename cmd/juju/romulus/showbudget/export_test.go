// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package showbudget

import (
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

var (
	NewBudgetAPIClient = &newBudgetAPIClient
	NewJujuclientStore = &newJujuclientStore
)

// BudgetAPIClientFnc returns a function that returns the provided budgetAPIClient
// and can be used to patch the NewBudgetAPIClient variable for tests.
func BudgetAPIClientFnc(api budgetAPIClient) func(*httpbakery.Client) (budgetAPIClient, error) {
	return func(*httpbakery.Client) (budgetAPIClient, error) {
		return api, nil
	}
}
