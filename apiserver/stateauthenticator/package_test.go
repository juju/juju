// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator_test

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package stateauthenticator -destination services_mock_test.go github.com/juju/juju/apiserver/stateauthenticator ControllerConfigService,UserService,AgentAuthenticatorFactory
//go:generate go run go.uber.org/mock/mockgen -typed -package stateauthenticator -destination authentication_mock_test.go github.com/juju/juju/apiserver/authentication EntityAuthenticator

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
