// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator_test

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package stateauthenticator_test -destination controller_config_mock_test.go github.com/juju/juju/apiserver/stateauthenticator ControllerConfigGetter
//go:generate go run go.uber.org/mock/mockgen -package stateauthenticator_test -destination user_domain_mock_test.go github.com/juju/juju/apiserver/stateauthenticator UserService

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
