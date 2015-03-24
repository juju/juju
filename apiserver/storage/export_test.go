// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

var (
	CreateAPI             = createAPI
	IsValidPoolListFilter = (*API).isValidPoolListFilter
	ValidateNames         = (*API).isValidNameCriteria
	ValidateProviders     = (*API).isValidProviderCriteria
)
