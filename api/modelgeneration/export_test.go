// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"github.com/juju/juju/api/base"
)

func NewStateFromCaller(caller base.FacadeCaller) *Client {
	return &Client{
		facade: caller,
	}
}
