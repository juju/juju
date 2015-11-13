// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

func IsServiceDirecotryAPIFacade(api ServiceDirectoryAPI) bool {
	_, ok := api.(*serviceDirectoryAPI)
	return ok
}
