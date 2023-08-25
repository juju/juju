// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/secrets/provider"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/http_mock.go net/http RoundTripper

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func MountPath(b provider.SecretsBackend) string {
	return b.(*vaultBackend).mountPath
}
