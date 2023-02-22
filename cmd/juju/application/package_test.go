// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	stdtesting "testing"

	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

// TODO(wallyworld) - convert tests moved across from commands package to not require mongo

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/applicationapi_mock.go github.com/juju/juju/cmd/juju/application ApplicationAPI,RemoveApplicationAPI
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/modelconfigapi_mock.go github.com/juju/juju/cmd/juju/application ModelConfigClient
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/deployer_mock.go github.com/juju/juju/cmd/juju/application/deployer Deployer,DeployerFactory

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
