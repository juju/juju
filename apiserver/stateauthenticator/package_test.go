// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator_test

import (
	"testing"

	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package stateauthenticator -destination services_mock_test.go github.com/juju/juju/apiserver/stateauthenticator ControllerConfigService,AccessService,MacaroonService,AgentAuthenticatorGetter,PasswordServiceGetter
//go:generate go run go.uber.org/mock/mockgen -typed -package stateauthenticator -destination authentication_mock_test.go github.com/juju/juju/apiserver/authentication EntityAuthenticator,PasswordService
//go:generate go run go.uber.org/mock/mockgen -typed -package stateauthenticator -destination expirable_storage_mock.go github.com/juju/juju/internal/macaroon ExpirableStorage

func TestPackage(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
