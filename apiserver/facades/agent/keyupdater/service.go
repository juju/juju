// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"context"

	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
)

// KeyUpdaterService is the interface for retrieving the authorised keys of a
// model.
type KeyUpdaterService interface {
	// AuthorisedKeysForMachine is responsible for fetching the authorised keys
	// that should be available on a machine. The following errors can be
	// expected:
	// - [github.com/juju/errors.NotValid] if the machine id is not valid.
	// - [github.com/juju/juju/domain/machine/errors.NotFound] if the machine does
	// not exist.
	AuthorisedKeysForMachine(context.Context, coremachine.Name) ([]string, error)

	// WatchAuthorisedKeysForMachine will watch for authorised key changes for a
	// give machine name. The following errors can be expected:
	// - [github.com/juju/errors.NotValid] if the machine id is not valid.
	WatchAuthorisedKeysForMachine(context.Context, coremachine.Name) (watcher.StringsWatcher, error)
}
