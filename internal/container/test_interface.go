// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

//go:generate go run go.uber.org/mock/mockgen -package testing -destination testing/interface_mock.go -write_package_comment=false github.com/juju/juju/internal/container TestLXDManager

// TestLXDManager for use in worker/provisioner tests
type TestLXDManager interface {
	Manager
	LXDProfileManager
	LXDProfileNameRetriever
}
