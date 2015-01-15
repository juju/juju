// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// Restore satisfies the Backups interface on windows.
func (b *backups) Restore(backupId string, args params.RestoreArgs) error {
	return errors.Errorf("backups not supported under windows")
}
