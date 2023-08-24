// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/jujuapi_mocks.go github.com/juju/juju/internal/secrets JujuAPIClient
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/backend_mocks.go github.com/juju/juju/internal/secrets/provider SecretsBackend

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
