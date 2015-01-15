// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
		"github.com/juju/juju/apiserver/params"
)

// Restore satisfies the Backups interface on windows.
func (b *backups) Restore(backupId string, args params.RestoreArgs) error {
	return nil
}