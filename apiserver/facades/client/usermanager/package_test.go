// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package usermanager_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/client/usermanager UserService

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
