// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/charmresource_mock.go github.com/juju/juju/cmd/juju/application/utils CharmClient
