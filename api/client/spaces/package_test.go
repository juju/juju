// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
)

func TestAll(t *stdtesting.T) {
	gc.TestingT(t)
}

func NewAPIFromCaller(caller base.FacadeCaller) *API {
	return &API{
		facade: caller,
	}
}
