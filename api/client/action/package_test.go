// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
)

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

func NewPrunerFromCaller(caller base.FacadeCaller) *Facade {
	return &Facade{
		facade: caller,
	}
}

func NewClientFromCaller(caller base.FacadeCaller) *Client {
	return &Client{
		facade: caller,
	}
}
