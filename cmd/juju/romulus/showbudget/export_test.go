// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package showbudget

import (
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

var (
	NewBudgetAPIClient = &newBudgetAPIClient
	NewAPIClient       = &newAPIClient
)

// APIClientFnc returns a function that returns the provided APIClient
// and can be used to patch the NewAPIClient variable in tests
func NewAPIClientFnc(api APIClient) func(*showBudgetCommand) (APIClient, error) {
	return func(*showBudgetCommand) (APIClient, error) {
		return api, nil
	}
}

// BudgetAPIClientFnc returns a function that returns the provided budgetAPIClient
// and can be used to patch the NewBudgetAPIClient variable for tests.
func BudgetAPIClientFnc(api budgetAPIClient) func(*httpbakery.Client) (budgetAPIClient, error) {
	return func(*httpbakery.Client) (budgetAPIClient, error) {
		return api, nil
	}
}
