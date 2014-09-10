// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/backups"
)

var (
	NewBackupsStorage = &newBackupsStorage
)

func SetImpl(api *API, impl backups.Backups) {
	api.backups = impl
}

func APIValues(api *API) (*state.State, backups.Backups) {
	return api.st, api.backups
}
