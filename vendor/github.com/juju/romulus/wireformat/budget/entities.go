// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The budget package contains definitions of wireformats used by
// the budget service clients.
package budget

import (
	"strings"
)

// WalletWithBudgets represents the current state of the wallet and its budgets.
type WalletWithBudgets struct {
	Limit   string       `json:"limit, omitempty"`
	Total   WalletTotals `json:"total"`
	Budgets []Budget     `json:"budgets, omitempty"`
}

// SortedBudgets have additional methods that allow for sorting budgets.
type SortedBudgets []Budget

// Len is part of the sort.Interface implementation.
func (a SortedBudgets) Len() int {
	return len(a)
}

// Swap is part of the sort.Interface implementation.
func (a SortedBudgets) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// Less is part of the sort.Interface implementation.
func (a SortedBudgets) Less(i, j int) bool {
	return a[i].SortableKey() < a[j].SortableKey()
}

type WalletTotals struct {
	Limit       string `json:"limit, omitempty"`
	Budgeted    string `json:"budgeted"`
	Available   string `json:"available"`
	Unallocated string `json:"unallocated"`
	Usage       string `json:"usage"`
	Consumed    string `json:"consumed"`
}

// Budget represents the amount the user has allocated to a model.
type Budget struct {
	Owner    string `json:"owner"`
	Limit    string `json:"limit"`
	Consumed string `json:"consumed"`
	Usage    string `json:"usage"`
	Model    string `json:"model"`
}

// SortableKey returns a key by which allocations can be sorted.
func (a Budget) SortableKey() string {
	return a.Model
}

// ListWalletsResponse is returned by the ListBdugets API call.
type ListWalletsResponse struct {
	Wallets WalletSummaries `json:"wallets, omitempty"`
	Total   WalletTotals    `json:"total, omitempty"`
	Credit  string          `json:"credit, omitempty"`
}

// WalletSummaries is an alphabetically sorted list of wallet summaries.
type WalletSummaries []WalletSummary

// Implement sort.Interface.
func (b WalletSummaries) Len() int      { return len(b) }
func (b WalletSummaries) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b WalletSummaries) Less(i, j int) bool {
	return strings.ToLower(b[i].Wallet) < strings.ToLower(b[j].Wallet)
}

// WalletSummary represents the summary information for a single wallet in
// the ListWalletsResponse structure.
type WalletSummary struct {
	Owner       string `json:"owner"`
	Wallet      string `json:"wallet"`
	Limit       string `json:"limit"`
	Budgeted    string `json:"budgeted"`
	Unallocated string `json:"unallocated"`
	Available   string `json:"available"`
	Consumed    string `json:"consumed"`
	Default     bool   `json:"default,omitempty"`
}
