// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

func (b *API) Remove(args params.BackupsRemoveArgs) error {
	err := b.backups.Remove(args.ID)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
