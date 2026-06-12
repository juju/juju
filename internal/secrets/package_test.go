// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/jujuapi_mocks.go github.com/juju/juju/internal/secrets JujuAPIClient,SecretsState
//go:generate go run github.com/canonical/gomock/mockgen -package mocks -destination mocks/backend_mocks.go github.com/juju/juju/internal/secrets/provider SecretsBackend
