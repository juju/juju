// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault

import (
	"github.com/juju/juju/internal/secrets/provider"
)

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/http_mock.go net/http RoundTripper

func MountPath(b provider.SecretsBackend) string {
	return b.(*vaultBackend).mountPath
}
