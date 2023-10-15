// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/mocks.go github.com/juju/juju/apiserver/common/charms State,Application,Charm,Model

func Test(t *testing.T) {
	gc.TestingT(t)
}
