// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination testing/package_mock.go -write_package_comment=false github.com/juju/juju/internal/container Manager,Initialiser
//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination testing/interface_mock.go -write_package_comment=false github.com/juju/juju/internal/container TestLXDManager

func Test(t *testing.T) {
	gc.TestingT(t)
}
