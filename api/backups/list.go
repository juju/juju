// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// List implements the API method.
func (c *Client) List() (*params.BackupsListResult, error) {
	var result params.BackupsListResult
	args := params.BackupsListArgs{}
	if err := c.facade.FacadeCall("List", args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	return &result, nil
}
