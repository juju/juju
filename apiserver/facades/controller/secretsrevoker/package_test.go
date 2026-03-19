// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package secretsrevoker -destination mocks_test.go github.com/juju/juju/apiserver/facades/controller/secretsrevoker SecretsState,Getters
//go:generate go run go.uber.org/mock/mockgen -typed -package secretsrevoker -destination secretsprovider_mocks_test.go github.com/juju/juju/secrets/provider SecretBackendProvider

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
