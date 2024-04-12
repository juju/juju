// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import coresecrets "github.com/juju/juju/core/secrets"

// These type aliases are used to specify filter terms.
type (
	Labels            []string
	ApplicationOwners []string
	UnitOwners        []string
)

// These consts are used to specify nil filter terms.
var (
	NilLabels            = Labels(nil)
	NilApplicationOwners = ApplicationOwners(nil)
	NilUnitOwners        = UnitOwners(nil)
	NilRevision          = (*int)(nil)
	NilSecretURI         = (*coresecrets.URI)(nil)
)
