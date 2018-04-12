// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

var (
	ValidatePoolListFilter   = (*APIv4).validatePoolListFilter
	ValidateNameCriteria     = (*APIv4).validateNameCriteria
	ValidateProviderCriteria = (*APIv4).validateProviderCriteria
)
