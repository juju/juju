// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	stdtesting "testing"

	"github.com/juju/juju/v2/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/mocks.go github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater Application,CharmhubRefreshClient,Model,State

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
