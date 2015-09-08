// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

var (
	IsValidPoolListFilter = (*API).isValidPoolListFilter
	ValidateNames         = (*API).isValidNameCriteria
	ValidateProviders     = (*API).isValidProviderCriteria

	CreateAPI = createAPI
)
