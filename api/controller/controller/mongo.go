// Copyright 2012-2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/rpc/params"
)

// MongoVersion returns the mongo version associated with the state session.
func (c *Client) MongoVersion() (string, error) {
	var result params.StringResult
	err := c.facade.FacadeCall(context.TODO(), "MongoVersion", nil, &result)
	if err != nil {
		return "", errors.Trace(err)
	}
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}
