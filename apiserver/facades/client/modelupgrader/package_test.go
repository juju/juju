// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader

import (
	stdtesting "testing"

	"github.com/juju/juju/v2/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/modelupgrader StatePool,State,Model
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/agents_mock.go github.com/juju/juju/apiserver/common ToolsFinder
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/environs_mock.go github.com/juju/juju/environs BootstrapEnviron

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
