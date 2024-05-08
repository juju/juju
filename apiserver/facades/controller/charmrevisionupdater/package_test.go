// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/mocks.go github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater Application,CharmhubRefreshClient,Model,State
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/objectstore.go github.com/juju/juju/core/objectstore ObjectStore

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
