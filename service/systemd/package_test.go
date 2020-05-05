// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemd_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package systemd_test -destination package_mock_test.go github.com/juju/juju/service/systemd DBusAPI,FileSystemOps

// TODO (manadart 2020-01-28): Phase this out
// and generate all mocks with the command above.
//go:generate go run github.com/golang/mock/mockgen -package systemd -destination shims_mock_test.go github.com/juju/juju/service/systemd ShimExec

func Test(t *testing.T) {
	gc.TestingT(t)
}
