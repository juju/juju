// Copyright 2012-2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/params"
)

// MongoVersion returns the mongo version associated with the state session.
func (c *Client) MongoVersion() (string, error) {
	if c.BestAPIVersion() < 6 {
		return "", errors.NotSupportedf("MongoVersion not supported by this version of Juju")
	}
	var result params.StringResult
	err := c.facade.FacadeCall("MongoVersion", nil, &result)
	if err != nil {
		return "", errors.Trace(err)
	}
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}
