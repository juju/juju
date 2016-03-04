// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/juju/api/base/testing"
)

// PatchFacadeCall changes the internal FacadeCaller to an arbitrary call
// function for testing.
func PatchFacadeCall(p testing.Patcher, client *Client, call func(request string, params, response interface{}) error) {
	testing.PatchFacadeCall(p, &client.facade, call)
}
