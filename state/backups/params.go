// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/names"

	"github.com/juju/juju/instance"
)

// RestoreArgs holds the args to be used to call state/backups.Restore
type RestoreArgs struct {
	PrivateAddress string
	PublicAddress  string
	NewInstId      instance.Id
	NewInstTag     names.Tag
	NewInstSeries  string
}
