// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	stdtesting "testing"

	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

// TODO(wallyworld) - convert tests moved across from commands package to not require mongo

//go:generate go run github.com/golang/mock/mockgen -package application -destination status_mock_test.go github.com/juju/juju/cmd/juju/application StatusAPI

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
