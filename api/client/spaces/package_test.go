// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
)

func TestAll(t *stdtesting.T) {
	tc.TestingT(t)
}

func NewAPIFromCaller(caller base.FacadeCaller) *API {
	return &API{
		facade: caller,
	}
}
