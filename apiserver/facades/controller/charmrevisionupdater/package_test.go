// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/mocks.go github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater Application,CharmhubRefreshClient,Model,State

func TestAll(t *testing.T) {
	gc.TestingT(t)
}
