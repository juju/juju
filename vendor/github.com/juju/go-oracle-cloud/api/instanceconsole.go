// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"errors"
	"fmt"

	"github.com/juju/go-oracle-cloud/response"
)

// InstanceConsoleDetails retrieves the messages that
// appear when an instance boots. Use these messages
// to diagnose unresponsive instances and failures in
// the boot up process.
func (c *Client) InstanceConsoleDetails(
	name string,
) (resp response.InstanceConsole, err error) {

	if !c.isAuth() {
		return resp, errNotAuth
	}

	if name == "" {
		return resp, errors.New(
			"go-oracle-cloud: Empty instance console name",
		)
	}
	url := fmt.Sprintf("%s%s", c.endpoints["instanceconsole"], name)

	if err = c.request(paramsRequest{
		url:  url,
		verb: "GET",
		resp: &resp,
	}); err != nil {
		return resp, err
	}

	return resp, nil
}
