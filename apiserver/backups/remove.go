// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

func (a *API) Remove(args params.BackupsRemoveArgs) error {
	backups, closer := newBackups(a.st)
	defer closer.Close()

	err := backups.Remove(args.ID)
	return errors.Trace(err)
}
