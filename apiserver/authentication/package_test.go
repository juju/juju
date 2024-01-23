// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package authentication_test -destination domain_mock_test.go github.com/juju/juju/apiserver/authentication UserService

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
