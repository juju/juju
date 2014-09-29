// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

func (c *Client) Remove(id string) error {
	args := params.BackupsRemoveArgs{ID: id}
	if err := c.facade.FacadeCall("Remove", args, nil); err != nil {
		return errors.Trace(err)
	}
	return nil
}
