// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
)

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

func NewClientFromCaller(caller base.FacadeCaller) *Client {
	return &Client{
		facade: caller,
	}
}
