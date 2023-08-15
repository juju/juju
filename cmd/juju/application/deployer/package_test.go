// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/deploy_mock.go github.com/juju/juju/cmd/juju/application/deployer DeployerAPI,DeployStepAPI,CharmReader,DeployConfigFlag
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/deployer_mock.go github.com/juju/juju/cmd/juju/application/deployer ModelCommand,ConsumeDetails,ModelConfigGetter
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/resolver_mock.go github.com/juju/juju/cmd/juju/application/deployer Resolver,Bundle
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/api_mock.go github.com/juju/juju/api AllWatch
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/modelcmd_mock.go github.com/juju/juju/cmd/modelcmd Filesystem
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/write_mock.go io Writer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/charm_mock.go github.com/juju/charm/v11 Charm

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
