// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/secrets/provider"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/http_mock.go net/http RoundTripper

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

func MountPath(b provider.SecretsBackend) string {
	return b.(*vaultBackend).mountPath
}
