// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package space -destination environs_mock_test.go github.com/juju/juju/environs BootstrapEnviron,NetworkingEnviron
//go:generate go run go.uber.org/mock/mockgen -package space -destination spaces_mock_test.go github.com/juju/juju/environs/space ReloadSpacesState,Constraints

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
