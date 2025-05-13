// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/deploy_mock.go github.com/juju/juju/cmd/juju/application/deployer DeployerAPI,CharmReader,DeployConfigFlag
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/deployer_mock.go github.com/juju/juju/cmd/juju/application/deployer ModelCommand,ConsumeDetails,CharmDeployAPI
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/resolver_mock.go github.com/juju/juju/cmd/juju/application/deployer Resolver
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/modelcmd_mock.go github.com/juju/juju/cmd/modelcmd Filesystem
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/write_mock.go io Writer
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/charm_mock.go github.com/juju/juju/internal/charm Charm,Bundle

func TestPackage(t *testing.T) {
	tc.TestingT(t)
}
