// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actionpruner

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
