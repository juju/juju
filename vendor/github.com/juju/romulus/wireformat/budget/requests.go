// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package budget

// CreateWalletRequest is used in the requests to the budget service
// for creating the specified wallet.
type CreateWalletRequest struct {
	Wallet string `json:"wallet"`
	Limit  string `json:"limit"`
}

// ContentType return the content-type header to be set for the request.
func (CreateWalletRequest) ContentType() string { return "application/json" }

// Method returns the http method used for this request.
func (CreateWalletRequest) Method() string { return "POST" }

// Body returns the body of the request.
func (c CreateWalletRequest) Body() interface{} {
	return c
}

// URL returns the URL of the request.
func (CreateWalletRequest) URL(apiRoot string) string {
	return apiRoot + "/wallet"
}

// ListWalletsRequest defines a request to the budgets service
// to list a user's wallets.
type ListWalletsRequest struct{}

// Method returns the method of the request.
func (ListWalletsRequest) Method() string { return "GET" }

// URL returns the URL of the request.
func (ListWalletsRequest) URL(apiRoot string) string {
	return apiRoot + "/wallet"
}

// SetWalletRequest defines a request that updates the limit of
// a wallet.
type SetWalletRequest struct {
	Wallet string `json:"-"`
	Limit  string `json:"limit"`
}

// ContentType return the content-type header to be set for the request.
func (SetWalletRequest) ContentType() string { return "application/json" }

// Method returns the method of the request.
func (SetWalletRequest) Method() string { return "PATCH" }

// Body returns the request body.
func (r SetWalletRequest) Body() interface{} {
	return struct {
		Update SetWalletRequest `json:"update"`
	}{Update: r}
}

// URL returns the URL for the request.
func (r SetWalletRequest) URL(apiRoot string) string {
	return apiRoot + "/wallet/" + r.Wallet
}

// GetWalletRequest defines a request that retrieves a specific wallet.
type GetWalletRequest struct {
	Wallet string
}

// URL returns the URL for the request.
func (r GetWalletRequest) URL(apiRoot string) string {
	return apiRoot + "/wallet/" + r.Wallet
}

// Method returns the method for the request.
func (GetWalletRequest) Method() string { return "GET" }

// CreateBudgetRequest defines a request to create an budget in the specified wallet.
type CreateBudgetRequest struct {
	Model  string `json:"model"`
	Limit  string `json:"limit"`
	Wallet string `json:"-"`
}

// URL returns the URL for the request.
func (r CreateBudgetRequest) URL(apiRoot string) string {
	return apiRoot + "/wallet/" + r.Wallet + "/budget"
}

// ContentType return the content-type header to be set for the request.
func (CreateBudgetRequest) ContentType() string { return "application/json" }

// Method returns the method for the request.
func (CreateBudgetRequest) Method() string { return "POST" }

// Body returns the request body.
func (r CreateBudgetRequest) Body() interface{} { return r }

// UpdateBudgetRequest defines a request to update a budget
// associated with a model.
type UpdateBudgetRequest struct {
	Model  string `json:"-"`
	Limit  string `json:"limit,omitempty"`
	Wallet string `json:"wallet,omitempty"`
}

// ContentType return the content-type header to be set for the request.
func (UpdateBudgetRequest) ContentType() string { return "application/json" }

// URL returns the URL for the request.
func (r UpdateBudgetRequest) URL(apiRoot string) string {
	return apiRoot + "/model/" + r.Model + "/budget"
}

// Method returns the method for the request.
func (UpdateBudgetRequest) Method() string { return "PATCH" }

// Body returns the request body.
func (r UpdateBudgetRequest) Body() interface{} {
	return struct {
		Update UpdateBudgetRequest `json:"update"`
	}{Update: r}
}

// DeleteBudgetRequest defines a request that removes a budget associated
// with a model.
type DeleteBudgetRequest struct {
	Model string `json:"-"`
}

// URL returns the URL for the request.
func (r DeleteBudgetRequest) URL(apiRoot string) string {
	return apiRoot + "/model/" + r.Model + "/budget"
}

// Method returns the method for the request.
func (DeleteBudgetRequest) Method() string { return "DELETE" }
