// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The budget package contains definitions of wireformats used by
// the budget service clients.
package budget

import (
	"fmt"
	"sort"
	"strings"
)

// BudgetWithAllocations represents the current state of the budget and its allocations.
type BudgetWithAllocations struct {
	Limit       string       `json:"limit, omitempty"`
	Total       BudgetTotals `json:"total"`
	Allocations []Allocation `json:"allocations, omitempty"`
}

// SortedAllocations have additional methods that allow for sorting allocations.
type SortedAllocations []Allocation

// Len is part of the sort.Interface implementation.
func (a SortedAllocations) Len() int {
	return len(a)
}

// Swap is part of the sort.Interface implementation.
func (a SortedAllocations) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// Less is part of the sort.Interface implementation.
func (a SortedAllocations) Less(i, j int) bool {
	return a[i].SortableKey() < a[j].SortableKey()
}

type BudgetTotals struct {
	Limit       string `json:"limit, omitempty"`
	Allocated   string `json:"allocated"`
	Available   string `json:"available"`
	Unallocated string `json:"unallocated"`
	Usage       string `json:"usage"`
	Consumed    string `json:"consumed"`
}

// Allocation represents the amount the user has allocated to specific
// services in a named model.
type Allocation struct {
	Owner    string                       `json:"owner"`
	Limit    string                       `json:"limit"`
	Consumed string                       `json:"consumed"`
	Usage    string                       `json:"usage"`
	Model    string                       `json:"model"`
	Services map[string]ServiceAllocation `json:"services"`
}

// SortableKey returns a key by which allocations can be sorted.
func (a Allocation) SortableKey() string {
	if len(a.Services) == 0 {
		return a.Model
	} else {
		var services []string
		for svc := range a.Services {
			services = append(services, svc)
		}
		sort.Strings(services)
		return fmt.Sprintf("%s:%s", a.Model, services[0])
	}
}

// ServiceAllocation represents the amount the user
// has allocated to a specific service.
type ServiceAllocation struct {
	Consumed string `json:"consumed"`
}

// ListBudgetsResponse is returned by the ListBdugets API call.
type ListBudgetsResponse struct {
	Budgets BudgetSummaries `json:"budgets, omitempty"`
	Total   BudgetTotals    `json:"total, omitempty"`
	Credit  string          `json:"credit, omitempty"`
}

// BudgetSummaries is an alphabetically sorted list of budget summaries.
type BudgetSummaries []BudgetSummary

// Implement sort.Interface.
func (b BudgetSummaries) Len() int      { return len(b) }
func (b BudgetSummaries) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b BudgetSummaries) Less(i, j int) bool {
	return strings.ToLower(b[i].Budget) < strings.ToLower(b[j].Budget)
}

// BudgetSummary represents the summary information for a single budget in
// the ListBudgetsResponse structure.
type BudgetSummary struct {
	Owner       string `json:"owner"`
	Budget      string `json:"budget"`
	Limit       string `json:"limit"`
	Allocated   string `json:"allocated"`
	Unallocated string `json:"unallocated"`
	Available   string `json:"available"`
	Consumed    string `json:"consumed"`
}
