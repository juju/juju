// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination testing/package_mock.go -write_package_comment=false github.com/juju/juju/internal/container Manager,Initialiser
//go:generate go run go.uber.org/mock/mockgen -typed -package testing -destination testing/interface_mock.go -write_package_comment=false github.com/juju/juju/internal/container TestLXDManager

