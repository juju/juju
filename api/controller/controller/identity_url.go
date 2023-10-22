// Copyright 2012-2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/rpc/params"
)

// IdentityProviderURL returns the URL of the configured external identity
// provider for this controller or an empty string if no external identity
// provider has been configured when the controller was bootstrapped.
func (c *Client) IdentityProviderURL() (string, error) {
	var result params.StringResult
	err := c.facade.FacadeCall(context.TODO(), "IdentityProviderURL", nil, &result)
	if err != nil {
		return "", errors.Trace(err)
	}
	if result.Error != nil {
		return "", result.Error
	}
	return result.Result, nil
}
