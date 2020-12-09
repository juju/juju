// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package charmrevisionupdater_test -destination interface_mock_test.go github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater Application,Model,State

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
