// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

type Account struct {
	Credentials      map[string]string `json:"credentials,omitempty"`
	Description      string            `json:"description,omitempty"`
	Accounttype      string            `json:"accounttype,omitempty"`
	Name             string            `json:"name"`
	Uri              string            `json:"uri"`
	Objectproperties map[string]string `json:"objectproperties,omitempty"`
}

// DirectoryNames are names of all the accounts
// in the specified container.
type DirectoryNames struct {
	Result []string `json:"result,omitempty"`
}

// AllAccounts list of all accounts in the oracle cloud
type AllAccounts struct {
	Result []Account `json:"result,omitempty"`
}
