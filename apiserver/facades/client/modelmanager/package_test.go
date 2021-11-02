// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/apiserver/facades/client/modelmanager StatePool,State,Model
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/model_manager_backend_mock.go github.com/juju/juju/apiserver/common ModelManagerBackend

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
