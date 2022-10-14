// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func NewFacadeFromCaller(caller base.FacadeCaller) *Facade {
	return &Facade{
		caller: caller,
	}
}
