// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package services -destination interface_mocks_test.go github.com/juju/juju/internal/charm/services StateBackend,ModelBackend,Storage,UploadedCharm

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
