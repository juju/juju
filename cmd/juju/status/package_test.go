// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/mocks.go -source=status.go statusApi
func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
