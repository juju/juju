// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination testing/mocks_identity.go -write_package_comment=false github.com/juju/juju/internal/provider/oci IdentityClient
//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination testing/mocks_compute.go -write_package_comment=false github.com/juju/juju/internal/provider/oci ComputeClient
//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination testing/mocks_networking.go -write_package_comment=false github.com/juju/juju/internal/provider/oci NetworkingClient
//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination testing/mocks_firewall.go -write_package_comment=false github.com/juju/juju/internal/provider/oci FirewallClient
//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination testing/mocks_storage.go -write_package_comment=false github.com/juju/juju/internal/provider/oci StorageClient

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
