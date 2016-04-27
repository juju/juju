// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cachedimages

var (
	GetListImagesAPI  = &getListImagesAPI
	GetRemoveImageAPI = &getRemoveImageAPI

	NewRemoveCommandForTest = NewRemoveCommand
	NewListCommandForTest   = NewListCommand
)
