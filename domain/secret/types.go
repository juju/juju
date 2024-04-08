// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

// These type aliases are used to specify filter terms.
type (
	Labels            []string
	ApplicationOwners []string
	UnitOwners        []string
	Revisions         []int
)

// These consts are used to specify nil filter terms.
var (
	NilLabels            = Labels(nil)
	NilApplicationOwners = ApplicationOwners(nil)
	NilUnitOwners        = UnitOwners(nil)
	NilRevisions         = Revisions(nil)
)
