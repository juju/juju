// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

//go:generate go run github.com/canonical/gomock/mockgen -package testing -destination testing/package_mock.go -write_package_comment=false github.com/juju/juju/internal/container Manager,Initialiser
//go:generate go run github.com/canonical/gomock/mockgen -package testing -destination testing/interface_mock.go -write_package_comment=false github.com/juju/juju/internal/container TestLXDManager
