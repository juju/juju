// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"github.com/juju/errors"

	"github.com/juju/juju/rpc/params"
)

func (c *Client) Remove(ids ...string) ([]params.ErrorResult, error) {
	if len(ids) == 0 {
		return []params.ErrorResult{}, nil
	}
	args := params.BackupsRemoveArgs{IDs: ids}
	results := params.ErrorResults{}
	err := c.facade.FacadeCall("Remove", args, &results)
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf(
			"expected %d result(s), got %d",
			len(ids), len(results.Results),
		)
	}
	return results.Results, errors.Trace(err)
}
