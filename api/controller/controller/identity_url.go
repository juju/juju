// Copyright 2012-2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/errors"
	"github.com/juju/juju/v2/rpc/params"
)

// IdentityProviderURL returns the URL of the configured external identity
// provider for this controller or an empty string if no external identity
// provider has been configured when the controller was bootstrapped.
func (c *Client) IdentityProviderURL() (string, error) {
	if c.BestAPIVersion() < 7 {
		return "", errors.NotSupportedf("IdentityProviderURL not supported by this version of Juju")
	}
	var result params.StringResult
	err := c.facade.FacadeCall("IdentityProviderURL", nil, &result)
	if err != nil {
		return "", errors.Trace(err)
	}
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}
