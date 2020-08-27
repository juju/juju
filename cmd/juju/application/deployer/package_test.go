// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/deploy_mock.go github.com/juju/juju/cmd/juju/application/deployer DeployerAPI,DeployStepAPI
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/resolver_mock.go github.com/juju/juju/cmd/juju/application/deployer Resolver
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/api_mock.go github.com/juju/juju/api AllWatch
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/write_mock.go io Writer

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
