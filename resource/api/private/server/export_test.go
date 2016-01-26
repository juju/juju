// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

func ExposeUnitFacade(uf *UnitFacade) UnitDataStore {
	return uf.dataStore
}
