// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager

import (
	"github.com/juju/juju/api/base"
)

func NewClientFromCaller(caller base.FacadeCaller) *Client {
	return &Client{
		facade: caller,
	}
}
