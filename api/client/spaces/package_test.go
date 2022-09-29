// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	stdtesting "testing"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func NewAPIFromCaller(caller base.FacadeCaller) *API {
	return &API{
		facade: caller,
	}
}
