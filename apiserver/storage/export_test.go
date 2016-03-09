// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

var (
	ValidatePoolListFilter   = (*API).validatePoolListFilter
	ValidateNameCriteria     = (*API).validateNameCriteria
	ValidateProviderCriteria = (*API).validateProviderCriteria

	CreateAPI = createAPI
)
